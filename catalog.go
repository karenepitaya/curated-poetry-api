package poetryapi

import (
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	TypeFiveCharacter  = "五言绝句"
	TypeSevenCharacter = "七言绝句"

	defaultBaseEditionID   = "nlc-tang300-1933"
	alternateBasePoemID    = "tang-cui-hu-ti-du-cheng-nan-zhuang"
	alternateBaseEditionID = "benshi-shi-1933"
)

var (
	idPattern              = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	scanPagePattern        = regexp.MustCompile(`^[1-9][0-9]*(?:-[1-9][0-9]*)?$`)
	revisionPattern        = regexp.MustCompile(`^[1-9][0-9]*$`)
	sha256Pattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
	variantLocationPattern = regexp.MustCompile(`^line-[1-4]-char-[1-7]$`)
)

var requiredFallbackWorks = map[string]string{
	"李白\x00静夜思":   "床前明月光\x00疑是地上霜\x00举头望明月\x00低头思故乡",
	"王之涣\x00登鹳雀楼": "白日依山尽\x00黄河入海流\x00欲穷千里目\x00更上一层楼",
	"孟浩然\x00春晓":   "春眠不觉晓\x00处处闻啼鸟\x00夜来风雨声\x00花落知多少",
	"王维\x00相思":    "红豆生南国\x00春来发几枝\x00愿君多采撷\x00此物最相思",
	"柳宗元\x00江雪":   "千山鸟飞绝\x00万径人踪灭\x00孤舟蓑笠翁\x00独钓寒江雪",
	"王维\x00竹里馆":   "独坐幽篁里\x00弹琴复长啸\x00深林人不知\x00明月来相照",
	"贾岛\x00寻隐者不遇": "松下问童子\x00言师采药去\x00只在此山中\x00云深不知处",
	"崔护\x00题都城南庄": "去年今日此门中\x00人面桃花相映红\x00人面不知何处去\x00桃花依旧笑春风",
}

//go:embed data/poems.json data/evidence.json data/editions.json
var embeddedData embed.FS

type Poem struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	TitleTraditional  string   `json:"titleTraditional"`
	Author            string   `json:"author"`
	Dynasty           string   `json:"dynasty"`
	Type              string   `json:"type"`
	Verses            []string `json:"verses"`
	VersesTraditional []string `json:"versesTraditional"`
}

type Edition struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	Institution    string `json:"institution"`
	ScanURL        string `json:"scanUrl"`
	CommonsPageURL string `json:"commonsPageUrl"`
	RevisionID     string `json:"revisionId"`
	SHA256         string `json:"sha256"`
	License        string `json:"license"`
	AccessedAt     string `json:"accessedAt"`
}

type Witness struct {
	EditionID    string   `json:"editionId"`
	ScanPage     string   `json:"scanPage"`
	PrintedFolio string   `json:"printedFolio"`
	Verses       []string `json:"verses"`
}

type VariantReading struct {
	EditionID string `json:"editionId"`
	Text      string `json:"text"`
}

type Variant struct {
	Location  string           `json:"location"`
	Readings  []VariantReading `json:"readings"`
	Chosen    string           `json:"chosen"`
	Rationale string           `json:"rationale"`
}

type Normalization struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

type PoemEvidence struct {
	PoemID         string          `json:"poemId"`
	Status         string          `json:"status"`
	Witnesses      []Witness       `json:"witnesses"`
	Variants       []Variant       `json:"variants"`
	Normalizations []Normalization `json:"normalizations"`
	ReviewedAt     string          `json:"reviewedAt"`
	ReviewMethod   string          `json:"reviewMethod"`
}

type poemFile struct {
	Poems []Poem `json:"poems"`
}

type evidenceFile struct {
	Poems []PoemEvidence `json:"poems"`
}

type editionFile struct {
	Editions []Edition `json:"editions"`
}

// Catalog is immutable after loading and is safe for concurrent use.
type Catalog struct {
	poems    []Poem
	byType   map[string][]int
	evidence []PoemEvidence
	editions []Edition
}

// Load validates and loads the data embedded in the executable. Invalid or
// incomplete curation data is returned as an error so callers can fail closed.
func Load() (*Catalog, error) {
	return LoadFS(embeddedData)
}

// LoadFS exists to make the complete load-and-validation path testable.
func LoadFS(dataFS fs.FS) (*Catalog, error) {
	var poems poemFile
	if err := decodeJSONFile(dataFS, "data/poems.json", &poems); err != nil {
		return nil, err
	}
	var evidence evidenceFile
	if err := decodeJSONFile(dataFS, "data/evidence.json", &evidence); err != nil {
		return nil, err
	}
	var editions editionFile
	if err := decodeJSONFile(dataFS, "data/editions.json", &editions); err != nil {
		return nil, err
	}
	if err := validate(poems.Poems, evidence.Poems, editions.Editions); err != nil {
		return nil, fmt.Errorf("validate poetry data: %w", err)
	}

	byType := map[string][]int{
		TypeFiveCharacter:  make([]int, 0, 25),
		TypeSevenCharacter: make([]int, 0, 25),
	}
	for i := range poems.Poems {
		byType[poems.Poems[i].Type] = append(byType[poems.Poems[i].Type], i)
	}
	return &Catalog{
		poems:    append([]Poem(nil), poems.Poems...),
		byType:   byType,
		evidence: append([]PoemEvidence(nil), evidence.Poems...),
		editions: append([]Edition(nil), editions.Editions...),
	}, nil
}

func decodeJSONFile(dataFS fs.FS, name string, target any) error {
	f, err := dataFS.Open(name)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: multiple JSON values", name)
		}
		return fmt.Errorf("decode %s trailing content: %w", name, err)
	}
	return nil
}

func (c *Catalog) Count() int {
	return len(c.poems)
}

func (c *Catalog) Poems() []Poem {
	result := make([]Poem, len(c.poems))
	for i := range c.poems {
		result[i] = clonePoem(c.poems[i])
	}
	return result
}

func clonePoem(p Poem) Poem {
	p.Verses = append([]string(nil), p.Verses...)
	p.VersesTraditional = append([]string(nil), p.VersesTraditional...)
	return p
}

func (c *Catalog) Random(typeName string) (Poem, error) {
	if typeName == "" {
		index, err := randomIndex(len(c.poems))
		if err != nil {
			return Poem{}, err
		}
		return clonePoem(c.poems[index]), nil
	}
	indices, ok := c.byType[typeName]
	if !ok {
		return Poem{}, fmt.Errorf("unsupported poem type %q", typeName)
	}
	index, err := randomIndex(len(indices))
	if err != nil {
		return Poem{}, err
	}
	return clonePoem(c.poems[indices[index]]), nil
}

func randomIndex(length int) (int, error) {
	if length <= 0 {
		return 0, errors.New("no poems available")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, fmt.Errorf("choose random poem: %w", err)
	}
	return int(n.Int64()), nil
}

func validate(poems []Poem, evidence []PoemEvidence, editions []Edition) error {
	var problems []string
	if len(poems) != 50 {
		problems = append(problems, fmt.Sprintf("expected exactly 50 poems, got %d", len(poems)))
	}

	typeCounts := map[string]int{}
	authorCounts := map[string]int{}
	ids := map[string]struct{}{}
	works := map[string]struct{}{}
	contents := map[string]struct{}{}
	poemByID := map[string]Poem{}
	for i, poem := range poems {
		label := fmt.Sprintf("poem[%d]", i)
		if poem.ID == "" || !idPattern.MatchString(poem.ID) {
			problems = append(problems, label+": invalid id")
		}
		if _, exists := ids[poem.ID]; exists {
			problems = append(problems, label+": duplicate id "+poem.ID)
		}
		ids[poem.ID] = struct{}{}
		poemByID[poem.ID] = poem
		if !isHanText(poem.Title) || !isHanText(poem.TitleTraditional) {
			problems = append(problems, label+": title must contain only Han characters")
		}
		if titleLength := utf8.RuneCountInString(poem.Title); titleLength < 2 || titleLength > 11 || poem.Title == "句" {
			problems = append(problems, label+": title does not satisfy blog layout constraints")
		}
		if traditionalTitleLength := utf8.RuneCountInString(poem.TitleTraditional); traditionalTitleLength < 2 || traditionalTitleLength > 11 || poem.TitleTraditional == "句" {
			problems = append(problems, label+": traditional title does not satisfy layout constraints")
		}
		if !isHanText(poem.Author) || poem.Author == "佚名" {
			problems = append(problems, label+": missing or anonymous author")
		}
		if poem.Dynasty != "唐" {
			problems = append(problems, label+": dynasty must be 唐")
		}
		expectedCharacters := 0
		switch poem.Type {
		case TypeFiveCharacter:
			expectedCharacters = 5
		case TypeSevenCharacter:
			expectedCharacters = 7
		default:
			problems = append(problems, label+": unsupported type "+poem.Type)
		}
		typeCounts[poem.Type]++
		authorCounts[poem.Author]++
		problems = append(problems, validateVerses(label+".verses", poem.Verses, expectedCharacters)...)
		problems = append(problems, validateVerses(label+".versesTraditional", poem.VersesTraditional, expectedCharacters)...)

		workKey := poem.Author + "\x00" + poem.Title
		if _, exists := works[workKey]; exists {
			problems = append(problems, label+": duplicate author/title")
		}
		works[workKey] = struct{}{}
		contentKey := strings.Join(poem.Verses, "\x00")
		if _, exists := contents[contentKey]; exists {
			problems = append(problems, label+": duplicate content")
		}
		contents[contentKey] = struct{}{}
	}
	for _, typeName := range []string{TypeFiveCharacter, TypeSevenCharacter} {
		if typeCounts[typeName] != 25 {
			problems = append(problems, fmt.Sprintf("expected 25 %s poems, got %d", typeName, typeCounts[typeName]))
		}
	}
	for author, count := range authorCounts {
		if count > 4 {
			problems = append(problems, fmt.Sprintf("author %s exceeds limit: %d", author, count))
		}
	}
	for workKey, expectedContent := range requiredFallbackWorks {
		if _, ok := works[workKey]; !ok {
			problems = append(problems, "missing required blog fallback poem "+strings.ReplaceAll(workKey, "\x00", "/"))
			continue
		}
		parts := strings.SplitN(workKey, "\x00", 2)
		for _, poem := range poems {
			if poem.Author == parts[0] && poem.Title == parts[1] && strings.Join(poem.Verses, "\x00") != expectedContent {
				problems = append(problems, "required blog fallback text differs for "+parts[0]+"/"+parts[1])
			}
		}
	}

	editionByID := map[string]Edition{}
	for i, edition := range editions {
		label := fmt.Sprintf("edition[%d]", i)
		if strings.TrimSpace(edition.ID) == "" {
			problems = append(problems, label+": missing id")
		}
		if _, exists := editionByID[edition.ID]; exists {
			problems = append(problems, label+": duplicate id "+edition.ID)
		}
		editionByID[edition.ID] = edition
		if strings.TrimSpace(edition.Title) == "" || edition.Year <= 0 || strings.TrimSpace(edition.Institution) == "" {
			problems = append(problems, label+": incomplete bibliographic metadata")
		}
		if !isHTTPSURL(edition.ScanURL) || !isHTTPSURL(edition.CommonsPageURL) || strings.TrimSpace(edition.License) == "" {
			problems = append(problems, label+": missing or invalid scan URL, Commons page URL, or license")
		}
		if _, err := time.Parse("2006-01-02", edition.AccessedAt); err != nil {
			problems = append(problems, label+": accessedAt must be YYYY-MM-DD")
		}
		if strings.TrimSpace(edition.RevisionID) == "" && strings.TrimSpace(edition.SHA256) == "" {
			problems = append(problems, label+": revisionId or sha256 is required")
		}
		if edition.RevisionID != "" && !revisionPattern.MatchString(edition.RevisionID) {
			problems = append(problems, label+": revisionId must be a positive numeric MediaWiki revision")
		}
		if edition.SHA256 != "" && !sha256Pattern.MatchString(edition.SHA256) {
			problems = append(problems, label+": sha256 must be 64 lowercase hexadecimal characters")
		}
	}

	evidenceByPoem := map[string]PoemEvidence{}
	for i, item := range evidence {
		label := fmt.Sprintf("evidence[%d]", i)
		if _, exists := evidenceByPoem[item.PoemID]; exists {
			problems = append(problems, label+": duplicate poemId "+item.PoemID)
		}
		evidenceByPoem[item.PoemID] = item
		poem, exists := poemByID[item.PoemID]
		if !exists {
			problems = append(problems, label+": unknown poemId "+item.PoemID)
		}
		if item.Status != "verified" {
			problems = append(problems, label+": status must be verified")
		}
		if item.Variants == nil || item.Normalizations == nil {
			problems = append(problems, label+": variants and normalizations must be explicit arrays")
		}
		if len(item.Witnesses) < 2 {
			problems = append(problems, label+": at least two witnesses are required")
		}
		witnessEditions := map[string]struct{}{}
		hasCorroboratingWitness := false
		for j, witness := range item.Witnesses {
			wlabel := fmt.Sprintf("%s.witnesses[%d]", label, j)
			edition, ok := editionByID[witness.EditionID]
			if !ok {
				problems = append(problems, wlabel+": unknown editionId "+witness.EditionID)
			} else if edition.Year == 1705 && strings.HasPrefix(edition.ID, "qts-1705-") {
				hasCorroboratingWitness = true
			}
			if _, exists := witnessEditions[witness.EditionID]; exists {
				problems = append(problems, wlabel+": repeated editionId "+witness.EditionID)
			}
			witnessEditions[witness.EditionID] = struct{}{}
			if !isValidScanPage(witness.ScanPage) {
				problems = append(problems, wlabel+": scanPage must be a positive 1-based page or ascending page range")
			}
			if strings.TrimSpace(witness.PrintedFolio) == "" {
				problems = append(problems, wlabel+": missing printedFolio")
			}
			expectedCharacters := 0
			if poem.Type == TypeFiveCharacter {
				expectedCharacters = 5
			} else if poem.Type == TypeSevenCharacter {
				expectedCharacters = 7
			}
			problems = append(problems, validateVerses(wlabel+".verses", witness.Verses, expectedCharacters)...)
		}
		baseEditionID := requiredBaseEditionID(item.PoemID)
		if _, ok := witnessEditions[baseEditionID]; !ok {
			problems = append(problems, fmt.Sprintf("%s: missing required base edition %s", label, baseEditionID))
		}
		if !hasCorroboratingWitness {
			problems = append(problems, label+": missing 1705 corroborating witness")
		}
		variantByLocation := map[string]Variant{}
		witnessEditionIDs := map[string]struct{}{}
		for _, witness := range item.Witnesses {
			witnessEditionIDs[witness.EditionID] = struct{}{}
		}
		for j, variant := range item.Variants {
			vlabel := fmt.Sprintf("%s.variants[%d]", label, j)
			if _, duplicate := variantByLocation[variant.Location]; duplicate {
				problems = append(problems, vlabel+": duplicate location "+variant.Location)
			}
			variantByLocation[variant.Location] = variant
			if strings.TrimSpace(variant.Location) == "" || strings.TrimSpace(variant.Chosen) == "" || strings.TrimSpace(variant.Rationale) == "" {
				problems = append(problems, vlabel+": incomplete variant decision")
			}
			if !variantLocationPattern.MatchString(variant.Location) {
				problems = append(problems, vlabel+": location must be line-N-char-M")
			}
			if utf8.RuneCountInString(variant.Chosen) != 1 || !isHanText(variant.Chosen) {
				problems = append(problems, vlabel+": chosen must be one Han character")
			}
			if len(variant.Readings) < 2 {
				problems = append(problems, vlabel+": at least two readings are required")
			}
			seenReadings := map[string]struct{}{}
			for k, reading := range variant.Readings {
				if _, ok := editionByID[reading.EditionID]; !ok || strings.TrimSpace(reading.Text) == "" {
					problems = append(problems, fmt.Sprintf("%s.readings[%d]: invalid edition or empty text", vlabel, k))
				}
				if _, ok := witnessEditionIDs[reading.EditionID]; !ok {
					problems = append(problems, fmt.Sprintf("%s.readings[%d]: edition is not a witness for this poem", vlabel, k))
				}
				if _, duplicate := seenReadings[reading.EditionID]; duplicate {
					problems = append(problems, fmt.Sprintf("%s.readings[%d]: duplicate editionId", vlabel, k))
				}
				seenReadings[reading.EditionID] = struct{}{}
				if utf8.RuneCountInString(reading.Text) != 1 || !isHanText(reading.Text) {
					problems = append(problems, fmt.Sprintf("%s.readings[%d]: text must be one Han character", vlabel, k))
				}
			}
		}
		for j, normalization := range item.Normalizations {
			if strings.TrimSpace(normalization.From) == "" || strings.TrimSpace(normalization.To) == "" || strings.TrimSpace(normalization.Reason) == "" {
				problems = append(problems, fmt.Sprintf("%s.normalizations[%d]: incomplete normalization", label, j))
			}
			if normalization.From == normalization.To {
				problems = append(problems, fmt.Sprintf("%s.normalizations[%d]: from and to must differ", label, j))
			}
			if utf8.RuneCountInString(normalization.From) != 1 || utf8.RuneCountInString(normalization.To) != 1 || !isHanText(normalization.From) || !isHanText(normalization.To) {
				problems = append(problems, fmt.Sprintf("%s.normalizations[%d]: from and to must each be one Han character", label, j))
			}
		}
		if exists {
			problems = append(problems, validateVariantCoverage(label, poem, item.Witnesses, variantByLocation)...)
			problems = append(problems, validateNormalizationCoverage(label, poem, item.Normalizations)...)
		}
		if _, err := time.Parse("2006-01-02", item.ReviewedAt); err != nil {
			problems = append(problems, label+": reviewedAt must be YYYY-MM-DD")
		}
		if strings.TrimSpace(item.ReviewMethod) == "" {
			problems = append(problems, label+": missing reviewMethod")
		}
	}
	for id := range poemByID {
		if _, ok := evidenceByPoem[id]; !ok {
			problems = append(problems, "missing evidence for "+id)
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "; "))
}

func validateVariantCoverage(label string, poem Poem, witnesses []Witness, variants map[string]Variant) []string {
	var problems []string
	if len(poem.VersesTraditional) != 4 {
		return problems
	}
	for lineIndex, selectedVerse := range poem.VersesTraditional {
		selectedRunes := []rune(selectedVerse)
		for characterIndex, selected := range selectedRunes {
			readings := map[string]string{}
			differs := false
			complete := true
			for _, witness := range witnesses {
				if len(witness.Verses) != 4 {
					complete = false
					continue
				}
				witnessRunes := []rune(witness.Verses[lineIndex])
				if len(witnessRunes) != len(selectedRunes) {
					complete = false
					continue
				}
				reading := string(witnessRunes[characterIndex])
				readings[witness.EditionID] = reading
				if witnessRunes[characterIndex] != selected {
					differs = true
				}
			}
			if !complete || !differs {
				continue
			}
			location := fmt.Sprintf("line-%d-char-%d", lineIndex+1, characterIndex+1)
			variant, ok := variants[location]
			if !ok {
				problems = append(problems, label+": missing variant decision for "+location)
				continue
			}
			if variant.Chosen != string(selected) {
				problems = append(problems, label+": variant "+location+" chosen text does not match versesTraditional")
			}
			recorded := map[string]string{}
			for _, reading := range variant.Readings {
				recorded[reading.EditionID] = reading.Text
			}
			for editionID, reading := range readings {
				if recorded[editionID] != reading {
					problems = append(problems, fmt.Sprintf("%s: variant %s does not record %s reading %q", label, location, editionID, reading))
				}
			}
		}
	}
	return problems
}

func requiredBaseEditionID(poemID string) string {
	if poemID == alternateBasePoemID {
		return alternateBaseEditionID
	}
	return defaultBaseEditionID
}

func validateNormalizationCoverage(label string, poem Poem, normalizations []Normalization) []string {
	var problems []string
	if len(poem.Verses) != 4 || len(poem.VersesTraditional) != 4 {
		return problems
	}
	recorded := map[string]struct{}{}
	for _, normalization := range normalizations {
		recorded[normalization.From+"\x00"+normalization.To] = struct{}{}
	}
	for lineIndex := range poem.Verses {
		simplified := []rune(poem.Verses[lineIndex])
		traditional := []rune(poem.VersesTraditional[lineIndex])
		if len(simplified) != len(traditional) {
			continue
		}
		for characterIndex := range simplified {
			if simplified[characterIndex] == traditional[characterIndex] {
				continue
			}
			key := string(traditional[characterIndex]) + "\x00" + string(simplified[characterIndex])
			if _, ok := recorded[key]; !ok {
				problems = append(problems, fmt.Sprintf("%s: missing normalization %q to %q at line-%d-char-%d", label, traditional[characterIndex], simplified[characterIndex], lineIndex+1, characterIndex+1))
			}
		}
	}
	if simplifiedTitle, traditionalTitle := []rune(poem.Title), []rune(poem.TitleTraditional); len(simplifiedTitle) == len(traditionalTitle) {
		for characterIndex := range simplifiedTitle {
			if simplifiedTitle[characterIndex] == traditionalTitle[characterIndex] {
				continue
			}
			key := string(traditionalTitle[characterIndex]) + "\x00" + string(simplifiedTitle[characterIndex])
			if _, ok := recorded[key]; !ok {
				problems = append(problems, fmt.Sprintf("%s: missing title normalization %q to %q at title-char-%d", label, traditionalTitle[characterIndex], simplifiedTitle[characterIndex], characterIndex+1))
			}
		}
	}
	return problems
}

func isHanText(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == utf8.RuneError || !unicode.Is(unicode.Han, r) || unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Co, r) {
			return false
		}
	}
	return true
}

func isHTTPSURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func isValidScanPage(value string) bool {
	if !scanPagePattern.MatchString(value) {
		return false
	}
	parts := strings.Split(value, "-")
	if len(parts) == 1 {
		return true
	}
	start, startErr := strconv.Atoi(parts[0])
	end, endErr := strconv.Atoi(parts[1])
	return startErr == nil && endErr == nil && end > start
}

func validateVerses(label string, verses []string, expectedCharacters int) []string {
	var problems []string
	if len(verses) != 4 {
		return []string{fmt.Sprintf("%s: expected 4 verses, got %d", label, len(verses))}
	}
	for i, verse := range verses {
		vlabel := fmt.Sprintf("%s[%d]", label, i)
		if !utf8.ValidString(verse) {
			problems = append(problems, vlabel+": invalid UTF-8")
			continue
		}
		if utf8.RuneCountInString(verse) != expectedCharacters {
			problems = append(problems, fmt.Sprintf("%s: expected %d characters, got %d", vlabel, expectedCharacters, utf8.RuneCountInString(verse)))
		}
		for _, r := range verse {
			if r == utf8.RuneError || !unicode.Is(unicode.Han, r) || unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Co, r) {
				problems = append(problems, fmt.Sprintf("%s: invalid character %q", vlabel, r))
			}
		}
	}
	return problems
}
