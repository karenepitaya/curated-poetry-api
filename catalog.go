package poetryapi

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"path"
	"sort"
	"strings"
	"unicode"
)

var ErrNoMatchingWorks = errors.New("no works match the query")

//go:embed corpus
var embeddedCorpus embed.FS

// Catalog is immutable after loading and safe for concurrent use. The random
// reader is only replaceable by package tests before concurrent access begins.
type Catalog struct {
	works          []Work
	editions       []Edition
	collections    []Collection
	normalizations []NormalizationRule
	stats          CorpusStats
	randomReader   io.Reader
}

// Load validates and loads the corpus embedded in the executable.
func Load() (*Catalog, error) {
	return LoadFS(embeddedCorpus)
}

// LoadFS exposes the complete loading and validation path for tests and tools.
// dataFS must contain the corpus directory at its root.
func LoadFS(dataFS fs.FS) (*Catalog, error) {
	data, err := loadCorpus(dataFS, nil)
	if err != nil {
		return nil, err
	}
	if err := validateCorpus(data, false); err != nil {
		return nil, fmt.Errorf("validate poetry corpus: %w", err)
	}
	return newCatalog(data), nil
}

// CheckFilesFS validates only the listed work records while still loading the
// shared editions, collections and normalization rules they depend on. A change
// to shared metadata deliberately falls back to full validation. Missing paths
// are errors; callers handling deleted files must explicitly request a full check.
func CheckFilesFS(dataFS fs.FS, files []string) error {
	if len(files) == 0 {
		_, err := LoadFS(dataFS)
		return err
	}

	workPaths := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, name := range files {
		name = strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		if _, err := fs.Stat(dataFS, name); err != nil {
			return fmt.Errorf("stat %s: %w", name, err)
		}
		if !strings.HasPrefix(name, "corpus/works/") || path.Ext(name) != ".json" {
			_, err := LoadFS(dataFS)
			return err
		}
		workPaths = append(workPaths, name)
	}
	if len(workPaths) == 0 {
		_, err := LoadFS(dataFS)
		return err
	}

	data, err := loadCorpus(dataFS, workPaths)
	if err != nil {
		return err
	}
	if err := validateCorpus(data, true); err != nil {
		return fmt.Errorf("validate poetry corpus files: %w", err)
	}
	if err := validateSelectedUniqueness(dataFS, workPaths); err != nil {
		return fmt.Errorf("validate poetry corpus uniqueness: %w", err)
	}
	return nil
}

func validateSelectedUniqueness(dataFS fs.FS, selectedPaths []string) error {
	allPaths, err := jsonPaths(dataFS, "corpus/works")
	if err != nil {
		return err
	}
	selected := make(map[string]struct{}, len(selectedPaths))
	for _, name := range selectedPaths {
		selected[name] = struct{}{}
	}
	ids := make(map[string]string, len(allPaths))
	workKeys := make(map[string]string, len(allPaths))
	contentKeys := make(map[string]string, len(allPaths))
	var problems []string
	for _, name := range allPaths {
		var work Work
		if err := decodeJSONFile(dataFS, name, &work); err != nil {
			return err
		}
		problems = appendSelectedDuplicate(problems, "id "+work.ID, ids[work.ID], name, selected)
		ids[work.ID] = name
		workKey := work.Author.Name.Hans + "\x00" + work.Title.Hans
		problems = appendSelectedDuplicate(problems, "author/title", workKeys[workKey], name, selected)
		workKeys[workKey] = name
		contentKey := workContentKey(work, ScriptHans)
		problems = appendSelectedDuplicate(problems, "content", contentKeys[contentKey], name, selected)
		contentKeys[contentKey] = name
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "; "))
}

func appendSelectedDuplicate(problems []string, kind, previous, current string, selected map[string]struct{}) []string {
	_, previousSelected := selected[previous]
	_, currentSelected := selected[current]
	if previous != "" && (previousSelected || currentSelected) {
		return append(problems, fmt.Sprintf("%s duplicates between %s and %s", kind, previous, current))
	}
	return problems
}

type corpusData struct {
	works          []Work
	workPaths      []string
	editions       []Edition
	collections    []Collection
	normalizations []NormalizationRule
	revision       string
}

type normalizationFile struct {
	Rules []NormalizationRule `json:"rules"`
}

func loadCorpus(dataFS fs.FS, selectedWorkPaths []string) (corpusData, error) {
	var data corpusData
	var err error

	if selectedWorkPaths == nil {
		data.workPaths, err = jsonPaths(dataFS, "corpus/works")
		if err != nil {
			return data, err
		}
	} else {
		data.workPaths = append([]string(nil), selectedWorkPaths...)
		sort.Strings(data.workPaths)
	}
	if len(data.workPaths) == 0 {
		return data, errors.New("corpus/works contains no JSON files")
	}
	for _, name := range data.workPaths {
		var work Work
		if err := decodeJSONFile(dataFS, name, &work); err != nil {
			return data, err
		}
		data.works = append(data.works, work)
	}

	editionPaths, err := jsonPaths(dataFS, "corpus/editions")
	if err != nil {
		return data, err
	}
	if len(editionPaths) == 0 {
		return data, errors.New("corpus/editions contains no JSON files")
	}
	for _, name := range editionPaths {
		var edition Edition
		if err := decodeJSONFile(dataFS, name, &edition); err != nil {
			return data, err
		}
		data.editions = append(data.editions, edition)
	}

	collectionPaths, err := jsonPaths(dataFS, "corpus/collections")
	if err != nil {
		return data, err
	}
	if len(collectionPaths) == 0 {
		return data, errors.New("corpus/collections contains no JSON files")
	}
	for _, name := range collectionPaths {
		var collection Collection
		if err := decodeJSONFile(dataFS, name, &collection); err != nil {
			return data, err
		}
		data.collections = append(data.collections, collection)
	}

	var normalizations normalizationFile
	if err := decodeJSONFile(dataFS, "corpus/normalization.json", &normalizations); err != nil {
		return data, err
	}
	data.normalizations = normalizations.Rules

	if selectedWorkPaths == nil {
		allPaths := make([]string, 0, len(data.workPaths)+len(editionPaths)+len(collectionPaths)+1)
		allPaths = append(allPaths, data.workPaths...)
		allPaths = append(allPaths, editionPaths...)
		allPaths = append(allPaths, collectionPaths...)
		allPaths = append(allPaths, "corpus/normalization.json")
		data.revision, err = corpusRevision(dataFS, allPaths)
		if err != nil {
			return data, err
		}
	}
	return data, nil
}

func jsonPaths(dataFS fs.FS, root string) ([]string, error) {
	var names []string
	err := fs.WalkDir(dataFS, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if path.Ext(name) != ".json" {
			return fmt.Errorf("unexpected non-JSON corpus file %s", name)
		}
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	sort.Strings(names)
	return names, nil
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

func corpusRevision(dataFS fs.FS, names []string) (string, error) {
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		content, err := fs.ReadFile(dataFS, name)
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", name, err)
		}
		_, _ = io.WriteString(hash, name)
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func newCatalog(data corpusData) *Catalog {
	catalog := &Catalog{
		works:          cloneWorks(data.works),
		editions:       append([]Edition(nil), data.editions...),
		collections:    cloneCollections(data.collections),
		normalizations: cloneNormalizations(data.normalizations),
		randomReader:   rand.Reader,
		stats: CorpusStats{
			Works:          len(data.works),
			ByDynasty:      map[string]int{DynastyTang: 0, DynastySong: 0},
			CorpusRevision: data.revision,
		},
	}
	for _, work := range catalog.works {
		catalog.stats.ByDynasty[work.Dynasty]++
	}
	return catalog
}

func (c *Catalog) Count() int { return len(c.works) }

func (c *Catalog) Stats() CorpusStats {
	return CorpusStats{
		Works:          c.stats.Works,
		ByDynasty:      cloneCounts(c.stats.ByDynasty),
		CorpusRevision: c.stats.CorpusRevision,
	}
}

func (c *Catalog) Works() []Work { return cloneWorks(c.works) }

func (c *Catalog) RandomWork(query Query) (Work, error) {
	if err := ValidateQuery(query); err != nil {
		return Work{}, err
	}
	indices := make([]int, 0, len(c.works))
	for i := range c.works {
		if matchesQuery(c.works[i], query) {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return Work{}, ErrNoMatchingWorks
	}
	selected, err := randomIndexFrom(c.randomReader, len(indices))
	if err != nil {
		return Work{}, err
	}
	return cloneWork(c.works[indices[selected]]), nil
}

func ValidateQuery(query Query) error {
	if query.Collection != "" && !validID(query.Collection) {
		return fmt.Errorf("invalid collection %q", query.Collection)
	}
	if query.Dynasty != "" && query.Dynasty != DynastyTang && query.Dynasty != DynastySong {
		return fmt.Errorf("invalid dynasty %q", query.Dynasty)
	}
	if query.Genre != "" && query.Genre != GenreShi && query.Genre != GenreCi {
		return fmt.Errorf("invalid genre %q", query.Genre)
	}
	if query.Form != "" && query.Form != FormGushi && query.Form != FormLushi && query.Form != FormJueju && query.Form != FormCi {
		return fmt.Errorf("invalid form %q", query.Form)
	}
	if query.Meter != "" && query.Meter != MeterFive && query.Meter != MeterSeven && query.Meter != MeterMixed {
		return fmt.Errorf("invalid meter %q", query.Meter)
	}
	if query.MaxChars < 0 || query.MaxChars > 5000 {
		return fmt.Errorf("max_chars must be between 1 and 5000")
	}
	if query.Script != "" && query.Script != ScriptHans && query.Script != ScriptHant {
		return fmt.Errorf("invalid script %q", query.Script)
	}
	return nil
}

func matchesQuery(work Work, query Query) bool {
	if query.Collection != "" && !workInCollection(work, query.Collection) {
		return false
	}
	if query.Dynasty != "" && work.Dynasty != query.Dynasty {
		return false
	}
	if query.Genre != "" && work.Genre != query.Genre {
		return false
	}
	if query.Form != "" && work.Form != query.Form {
		return false
	}
	if query.Meter != "" && work.Meter != query.Meter {
		return false
	}
	if query.MaxChars > 0 && workCharacterCount(work, query.Script) > query.MaxChars {
		return false
	}
	return true
}

func workInCollection(work Work, collectionID string) bool {
	for _, membership := range work.Collections {
		if membership.ID == collectionID {
			return true
		}
	}
	return false
}

func workCharacterCount(work Work, script Script) int {
	count := 0
	for _, section := range work.Sections {
		for _, line := range section.Lines {
			for _, r := range line.Text(script) {
				if !unicode.IsPunct(r) && !unicode.IsSpace(r) {
					count++
				}
			}
		}
	}
	return count
}

func randomIndexFrom(reader io.Reader, length int) (int, error) {
	if length <= 0 {
		return 0, errors.New("no works available")
	}
	n, err := rand.Int(reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, fmt.Errorf("choose random work: %w", err)
	}
	return int(n.Int64()), nil
}

func cloneWork(work Work) Work {
	if work.Tune != nil {
		tune := *work.Tune
		work.Tune = &tune
	}
	sections := work.Sections
	work.Sections = make([]Section, len(sections))
	for i, section := range sections {
		work.Sections[i] = section
		work.Sections[i].Lines = append([]Line(nil), section.Lines...)
	}
	work.Collections = append([]WorkCollection(nil), work.Collections...)
	for i := range work.Collections {
		work.Collections[i].Position = cloneInt(work.Collections[i].Position)
	}
	work.NormalizationOverrides = append([]NormalizationOverride(nil), work.NormalizationOverrides...)
	witnesses := work.Evidence.Witnesses
	work.Evidence.Witnesses = make([]Witness, len(witnesses))
	for i, witness := range witnesses {
		work.Evidence.Witnesses[i] = witness
		work.Evidence.Witnesses[i].Verses = append([]string(nil), witness.Verses...)
	}
	variants := work.Evidence.Variants
	work.Evidence.Variants = make([]Variant, len(variants))
	for i, variant := range variants {
		work.Evidence.Variants[i] = variant
		work.Evidence.Variants[i].Readings = append([]VariantReading(nil), variant.Readings...)
	}
	return work
}

func cloneWorks(works []Work) []Work {
	result := make([]Work, len(works))
	for i := range works {
		result[i] = cloneWork(works[i])
	}
	return result
}

func cloneCollections(collections []Collection) []Collection {
	result := make([]Collection, len(collections))
	for i, collection := range collections {
		result[i] = collection
		result[i].Members = append([]CollectionMember(nil), collection.Members...)
		for j := range result[i].Members {
			result[i].Members[j].Position = cloneInt(result[i].Members[j].Position)
		}
	}
	return result
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneNormalizations(rules []NormalizationRule) []NormalizationRule {
	result := make([]NormalizationRule, len(rules))
	for i, rule := range rules {
		result[i] = rule
		result[i].AuditedWorkIDs = append([]string(nil), rule.AuditedWorkIDs...)
	}
	return result
}

func cloneCounts(counts map[string]int) map[string]int {
	result := make(map[string]int, len(counts))
	for key, value := range counts {
		result[key] = value
	}
	return result
}
