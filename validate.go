package poetryapi

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	scanPagePattern = regexp.MustCompile(`^[1-9][0-9]*(?:-[1-9][0-9]*)?$`)
	revisionPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func validateCorpus(data corpusData, partial bool) error {
	var problems []string

	editionByID := make(map[string]Edition, len(data.editions))
	for i, edition := range data.editions {
		label := fmt.Sprintf("edition[%d]", i)
		problems = append(problems, validateEdition(label, edition)...)
		if _, duplicate := editionByID[edition.ID]; duplicate {
			problems = append(problems, label+": duplicate id "+edition.ID)
		}
		editionByID[edition.ID] = edition
	}

	collectionByID := make(map[string]Collection, len(data.collections))
	for i, collection := range data.collections {
		label := fmt.Sprintf("collection[%d]", i)
		problems = append(problems, validateCollection(label, collection, editionByID)...)
		if _, duplicate := collectionByID[collection.ID]; duplicate {
			problems = append(problems, label+": duplicate id "+collection.ID)
		}
		collectionByID[collection.ID] = collection
	}

	problems = append(problems, validateNormalizationRules(data.normalizations)...)

	workByID := make(map[string]Work, len(data.works))
	workKeys := make(map[string]string, len(data.works))
	contentKeys := make(map[string]string, len(data.works))
	for i, work := range data.works {
		label := fmt.Sprintf("work[%d]", i)
		if i < len(data.workPaths) {
			label = data.workPaths[i]
		}
		if previous, duplicate := workByID[work.ID]; duplicate {
			_ = previous
			problems = append(problems, label+": duplicate id "+work.ID)
		}
		workByID[work.ID] = work
		problems = append(problems, validateWork(label, work, editionByID, collectionByID, data.normalizations)...)
		if work.Evidence.Level == EvidenceDigitalTextChecked {
			problems = append(problems, validateDigitalRecord(label, work, data.digitalRecords)...)
		}

		workKey := work.Author.Name.Hans + "\x00" + work.Title.Hans
		if other, duplicate := workKeys[workKey]; duplicate {
			problems = append(problems, fmt.Sprintf("%s: duplicate author/title with %s", label, other))
		}
		workKeys[workKey] = label
		contentKey := workContentKey(work, ScriptHans)
		if other, duplicate := contentKeys[contentKey]; duplicate {
			problems = append(problems, fmt.Sprintf("%s: duplicate content with %s", label, other))
		}
		contentKeys[contentKey] = label

		if i < len(data.workPaths) {
			wantPath := "corpus/works/" + work.Dynasty + "/" + work.ID + ".json"
			if data.workPaths[i] != wantPath {
				problems = append(problems, fmt.Sprintf("%s: work path must be %s", label, wantPath))
			}
		}
	}

	for _, rule := range data.normalizations {
		for _, workID := range rule.AuditedWorkIDs {
			work, loaded := workByID[workID]
			if !loaded {
				if !partial {
					problems = append(problems, fmt.Sprintf("normalization %s->%s: unknown workId %s", rule.From, rule.To, workID))
				}
				continue
			}
			if !workUsesRule(work, rule) {
				problems = append(problems, fmt.Sprintf("normalization %s->%s: work %s does not use declared rule", rule.From, rule.To, workID))
			}
		}
	}

	if !partial {
		for _, collection := range data.collections {
			for _, member := range collection.Members {
				work, exists := workByID[member.WorkID]
				if !exists {
					problems = append(problems, fmt.Sprintf("collection %s: unknown member %s", collection.ID, member.WorkID))
					continue
				}
				if !hasMembership(work, collection.ID, member) {
					problems = append(problems, fmt.Sprintf("collection %s: member %s is missing matching work membership", collection.ID, member.WorkID))
				}
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "; "))
}

func validateEdition(label string, edition Edition) []string {
	var problems []string
	if edition.Kind == "digital-text" {
		if !validID(edition.ID) || strings.TrimSpace(edition.Title) == "" || strings.TrimSpace(edition.Institution) == "" || strings.TrimSpace(edition.License) == "" {
			problems = append(problems, label+": incomplete digital source metadata")
		}
		if !isHTTPSURL(edition.SourceURL) || !sha256Pattern.MatchString(edition.SHA256) || !validSourcePath(edition.SourcePath) {
			problems = append(problems, label+": digital source requires HTTPS URL, SHA-256 and source path")
		}
		if _, err := time.Parse("2006-01-02", edition.AccessedAt); err != nil {
			problems = append(problems, label+": accessedAt must be YYYY-MM-DD")
		}
		return problems
	}
	if edition.Kind != "" && edition.Kind != "scan" {
		problems = append(problems, label+": unsupported edition kind")
	}
	if strings.TrimSpace(edition.ID) == "" {
		problems = append(problems, label+": missing id")
	}
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
	return problems
}

func validateCollection(label string, collection Collection, editions map[string]Edition) []string {
	var problems []string
	if !validID(collection.ID) {
		problems = append(problems, label+": invalid id")
	}
	if !validLocalizedText(collection.Title, true) {
		problems = append(problems, label+": invalid title")
	}
	if collection.Status != "in-progress" && collection.Status != "complete" {
		problems = append(problems, label+": status must be in-progress or complete")
	}
	if _, exists := editions[collection.PrimaryEditionID]; !exists {
		problems = append(problems, label+": unknown primaryEditionId "+collection.PrimaryEditionID)
	}
	if collection.Members == nil {
		problems = append(problems, label+": members must be an explicit array")
	}
	if collection.ExpectedCount < 0 {
		problems = append(problems, label+": expectedCount must not be negative")
	}
	memberIDs := make(map[string]struct{}, len(collection.Members))
	positions := make(map[int]struct{}, len(collection.Members))
	lastPosition := 0
	for i, member := range collection.Members {
		memberLabel := fmt.Sprintf("%s.members[%d]", label, i)
		if !validID(member.WorkID) {
			problems = append(problems, memberLabel+": invalid workId")
		}
		if _, duplicate := memberIDs[member.WorkID]; duplicate {
			problems = append(problems, memberLabel+": duplicate workId "+member.WorkID)
		}
		memberIDs[member.WorkID] = struct{}{}
		problems = append(problems, validatePosition(memberLabel, member.PositionStatus, member.Position)...)
		if member.Position != nil {
			if _, duplicate := positions[*member.Position]; duplicate {
				problems = append(problems, memberLabel+": duplicate position "+strconv.Itoa(*member.Position))
			}
			positions[*member.Position] = struct{}{}
			if *member.Position <= lastPosition {
				problems = append(problems, memberLabel+": confirmed members must be in ascending position order")
			}
			lastPosition = *member.Position
		}
	}
	if collection.Status == "complete" {
		if collection.ExpectedCount <= 0 {
			problems = append(problems, label+": complete collection requires expectedCount")
		} else if len(collection.Members) != collection.ExpectedCount {
			problems = append(problems, fmt.Sprintf("%s: complete collection has %d members, expected %d", label, len(collection.Members), collection.ExpectedCount))
		}
		for i, member := range collection.Members {
			if member.PositionStatus != "confirmed" {
				problems = append(problems, fmt.Sprintf("%s.members[%d]: complete collection requires confirmed positions", label, i))
			} else if member.Position == nil || *member.Position != i+1 {
				problems = append(problems, fmt.Sprintf("%s.members[%d]: complete collection positions must be contiguous from 1", label, i))
			}
		}
	}
	return problems
}

func validateWork(label string, work Work, editions map[string]Edition, collections map[string]Collection, rules []NormalizationRule) []string {
	var problems []string
	if !validID(work.ID) {
		problems = append(problems, label+": invalid id")
	}
	if !validLocalizedText(work.Title, true) {
		problems = append(problems, label+": invalid title")
	}
	if !validLocalizedText(work.Author.Name, false) {
		problems = append(problems, label+": invalid author name")
	}
	if work.Author.AttributionStatus != "selected-edition" && work.Author.AttributionStatus != "unknown" && work.Author.AttributionStatus != "disputed" {
		problems = append(problems, label+": invalid attributionStatus")
	}
	if (work.Author.AttributionStatus == "unknown" || work.Author.AttributionStatus == "disputed") && strings.TrimSpace(work.Author.AttributionNote) == "" {
		problems = append(problems, label+": uncertain attribution requires attributionNote")
	}
	if work.Dynasty != DynastyTang && work.Dynasty != DynastySong {
		problems = append(problems, label+": invalid dynasty")
	}
	if work.Genre != GenreShi && work.Genre != GenreCi {
		problems = append(problems, label+": invalid genre")
	}
	if work.Form != FormGushi && work.Form != FormLushi && work.Form != FormJueju && work.Form != FormCi {
		problems = append(problems, label+": invalid form")
	}
	if work.Meter != MeterFive && work.Meter != MeterSeven && work.Meter != MeterMixed {
		problems = append(problems, label+": invalid meter")
	}
	if work.Genre == GenreCi {
		if work.Form != FormCi || work.Meter != MeterMixed || work.Tune == nil || !validLocalizedText(*work.Tune, true) {
			problems = append(problems, label+": ci requires form=ci, meter=mixed and a valid tune")
		}
	} else if work.Form == FormCi || work.Tune != nil {
		problems = append(problems, label+": shi must not use ci form or tune")
	}
	if (work.Form == FormJueju || work.Form == FormLushi) && work.Meter == MeterMixed {
		problems = append(problems, label+": jueju and lushi require meter=5 or meter=7")
	}
	if len(work.Sections) == 0 {
		problems = append(problems, label+": sections must not be empty")
	}

	lineByID := make(map[string]Line)
	sectionIDs := make(map[string]struct{}, len(work.Sections))
	stanzaLineCount := 0
	for sectionIndex, section := range work.Sections {
		sectionLabel := fmt.Sprintf("%s.sections[%d]", label, sectionIndex)
		if !validID(section.ID) {
			problems = append(problems, sectionLabel+": invalid id")
		}
		if _, duplicate := sectionIDs[section.ID]; duplicate {
			problems = append(problems, sectionLabel+": duplicate section id "+section.ID)
		}
		sectionIDs[section.ID] = struct{}{}
		if section.Kind != "preface" && section.Kind != "stanza" {
			problems = append(problems, sectionLabel+": kind must be preface or stanza")
		}
		if len(section.Lines) == 0 {
			problems = append(problems, sectionLabel+": lines must not be empty")
		}
		if section.Kind == "stanza" {
			stanzaLineCount += len(section.Lines)
		}
		for lineIndex, line := range section.Lines {
			lineLabel := fmt.Sprintf("%s.lines[%d]", sectionLabel, lineIndex)
			if !validID(line.ID) {
				problems = append(problems, lineLabel+": invalid id")
			}
			if _, duplicate := lineByID[line.ID]; duplicate {
				problems = append(problems, lineLabel+": duplicate line id "+line.ID)
			}
			lineByID[line.ID] = line
			if !validLiteraryText(line.Hans) || !validLiteraryText(line.Hant) {
				problems = append(problems, lineLabel+": text contains invalid or empty content")
			}
			if section.Kind == "stanza" && (work.Meter == MeterFive || work.Meter == MeterSeven) {
				want := 5
				if work.Meter == MeterSeven {
					want = 7
				}
				if got := countContentCharacters(line.Hans); got != want {
					problems = append(problems, fmt.Sprintf("%s.hans: got %d content characters, want %d", lineLabel, got, want))
				}
				if got := countContentCharacters(line.Hant); got != want {
					problems = append(problems, fmt.Sprintf("%s.hant: got %d content characters, want %d", lineLabel, got, want))
				}
			}
		}
	}
	if work.Form == FormJueju && stanzaLineCount != 4 {
		problems = append(problems, fmt.Sprintf("%s: jueju requires 4 stanza lines, got %d", label, stanzaLineCount))
	}
	if work.Form == FormLushi && stanzaLineCount != 8 {
		problems = append(problems, fmt.Sprintf("%s: lushi requires 8 stanza lines, got %d", label, stanzaLineCount))
	}

	if len(work.Collections) == 0 {
		problems = append(problems, label+": collections must not be empty")
	}
	seenCollections := make(map[string]struct{}, len(work.Collections))
	for i, membership := range work.Collections {
		membershipLabel := fmt.Sprintf("%s.collections[%d]", label, i)
		collection, exists := collections[membership.ID]
		if !exists {
			problems = append(problems, membershipLabel+": unknown collection "+membership.ID)
			continue
		}
		if _, duplicate := seenCollections[membership.ID]; duplicate {
			problems = append(problems, membershipLabel+": duplicate collection "+membership.ID)
		}
		seenCollections[membership.ID] = struct{}{}
		problems = append(problems, validatePosition(membershipLabel, membership.PositionStatus, membership.Position)...)
		if !collectionHasMember(collection, work.ID, membership) {
			problems = append(problems, membershipLabel+": collection manifest has no matching member")
		}
		if !hasWitness(work.Evidence.Witnesses, collection.PrimaryEditionID) && !(work.Evidence.DigitalSource != nil && work.Evidence.DigitalSource.EditionID == collection.PrimaryEditionID) {
			problems = append(problems, membershipLabel+": evidence lacks collection primary edition "+collection.PrimaryEditionID)
		}
	}

	problems = append(problems, validateEvidence(label+".evidence", work, lineByID, editions)...)
	if work.Evidence.Level != EvidenceDigitalTextChecked {
		problems = append(problems, validateNormalization(label, work, rules)...)
	}
	return problems
}

func validateEvidence(label string, work Work, lines map[string]Line, editions map[string]Edition) []string {
	var problems []string
	evidence := work.Evidence
	if evidence.Level == EvidenceDigitalTextChecked {
		source := evidence.DigitalSource
		if evidence.Status != "validated" || source == nil {
			return []string{label + ": digital text requires validated status and source locator"}
		}
		if edition, exists := editions[source.EditionID]; !exists || edition.Kind != "digital-text" || source.RecordIndex < 0 || source.Conversion != "opencc-python-reimplemented@0.1.7:s2t" {
			problems = append(problems, label+": invalid digital source or conversion")
		}
		if len(evidence.Witnesses) != 0 || len(evidence.Variants) != 0 || len(work.NormalizationOverrides) != 0 {
			problems = append(problems, label+": digital text must not claim scan witnesses or legacy normalization")
		}
		if _, err := time.Parse("2006-01-02", evidence.ReviewedAt); err != nil {
			problems = append(problems, label+": reviewedAt must be YYYY-MM-DD")
		}
		if strings.TrimSpace(evidence.ReviewMethod) == "" {
			problems = append(problems, label+": missing reviewMethod")
		}
		return problems
	}
	if evidence.DigitalSource != nil {
		problems = append(problems, label+": scan review must not use digital source evidence")
	}
	selectedLines := stanzaLines(work, ScriptHant)
	if evidence.Level != EvidencePrimaryScanReviewed {
		problems = append(problems, label+": unsupported evidence level")
	}
	if evidence.Status != "verified" {
		problems = append(problems, label+": only verified works may enter the runtime corpus")
	}
	if len(evidence.Witnesses) < 2 {
		problems = append(problems, label+": at least two witnesses are required")
	}
	witnessIDs := make(map[string]struct{}, len(evidence.Witnesses))
	for i, witness := range evidence.Witnesses {
		witnessLabel := fmt.Sprintf("%s.witnesses[%d]", label, i)
		if _, exists := editions[witness.EditionID]; !exists {
			problems = append(problems, witnessLabel+": unknown editionId "+witness.EditionID)
		}
		if _, duplicate := witnessIDs[witness.EditionID]; duplicate {
			problems = append(problems, witnessLabel+": duplicate editionId "+witness.EditionID)
		}
		witnessIDs[witness.EditionID] = struct{}{}
		if !isValidScanPage(witness.ScanPage) {
			problems = append(problems, witnessLabel+": invalid scanPage")
		}
		if strings.TrimSpace(witness.PrintedFolio) == "" {
			problems = append(problems, witnessLabel+": missing printedFolio")
		}
		if len(witness.Verses) == 0 {
			problems = append(problems, witnessLabel+": verses must not be empty")
		} else if len(witness.Verses) != len(selectedLines) {
			problems = append(problems, fmt.Sprintf("%s: got %d verses, want %d stanza lines", witnessLabel, len(witness.Verses), len(selectedLines)))
		}
		for j, verse := range witness.Verses {
			if !validWitnessText(verse) {
				problems = append(problems, fmt.Sprintf("%s.verses[%d]: invalid text", witnessLabel, j))
			}
		}
	}
	if evidence.Variants == nil {
		problems = append(problems, label+": variants must be an explicit array")
	}
	locationKeys := make(map[string]struct{}, len(evidence.Variants))
	for i, variant := range evidence.Variants {
		variantLabel := fmt.Sprintf("%s.variants[%d]", label, i)
		line, exists := lines[variant.Location.LineID]
		if !exists {
			problems = append(problems, variantLabel+": unknown lineId "+variant.Location.LineID)
			continue
		}
		lineText := []rune(trimTerminalPunctuation(line.Hant))
		if variant.Location.Start < 0 || variant.Location.End < variant.Location.Start || variant.Location.End > len(lineText) {
			problems = append(problems, variantLabel+": invalid zero-based half-open Unicode range")
			continue
		}
		locationKey := fmt.Sprintf("%s:%d:%d", variant.Location.LineID, variant.Location.Start, variant.Location.End)
		if _, duplicate := locationKeys[locationKey]; duplicate {
			problems = append(problems, variantLabel+": duplicate location")
		}
		locationKeys[locationKey] = struct{}{}
		if variant.Chosen != string(lineText[variant.Location.Start:variant.Location.End]) {
			problems = append(problems, variantLabel+": chosen text does not match selected traditional text")
		}
		if variant.Chosen != "" && !allHan(variant.Chosen) {
			problems = append(problems, variantLabel+": chosen must be Han text or empty for an omission")
		}
		if strings.TrimSpace(variant.Rationale) == "" {
			problems = append(problems, variantLabel+": missing rationale")
		}
		if len(variant.Readings) < 2 {
			problems = append(problems, variantLabel+": at least two readings are required")
		}
		seenReadings := make(map[string]struct{}, len(variant.Readings))
		allReadingsEmpty := variant.Chosen == ""
		for j, reading := range variant.Readings {
			readingLabel := fmt.Sprintf("%s.readings[%d]", variantLabel, j)
			if _, exists := witnessIDs[reading.EditionID]; !exists {
				problems = append(problems, readingLabel+": edition is not a work witness")
			}
			if _, duplicate := seenReadings[reading.EditionID]; duplicate {
				problems = append(problems, readingLabel+": duplicate editionId")
			}
			seenReadings[reading.EditionID] = struct{}{}
			if reading.Text != "" {
				allReadingsEmpty = false
				if !validWitnessText(reading.Text) {
					problems = append(problems, readingLabel+": text must be Han text or empty for an omission")
				}
			}
		}
		for witnessID := range witnessIDs {
			if _, exists := seenReadings[witnessID]; !exists {
				problems = append(problems, fmt.Sprintf("%s: missing reading for witness %s", variantLabel, witnessID))
			}
		}
		if allReadingsEmpty {
			problems = append(problems, variantLabel+": chosen and readings must not all be empty")
		}
	}
	problems = append(problems, validateVariantCoverage(label, work, evidence.Witnesses, evidence.Variants)...)
	if _, err := time.Parse("2006-01-02", evidence.ReviewedAt); err != nil {
		problems = append(problems, label+": reviewedAt must be YYYY-MM-DD")
	}
	if strings.TrimSpace(evidence.ReviewMethod) == "" {
		problems = append(problems, label+": missing reviewMethod")
	}
	return problems
}

func validateVariantCoverage(label string, work Work, witnesses []Witness, variants []Variant) []string {
	var problems []string
	selectedLines := stanzaLines(work, ScriptHant)
	if len(selectedLines) == 0 {
		return problems
	}
	for lineIndex, selectedLine := range selectedLines {
		selected := []rune(trimTerminalPunctuation(selectedLine.Text))
		lineVariants := validVariantsForLine(variants, selectedLine.ID, len(selected))
		for i := 1; i < len(lineVariants); i++ {
			previous := lineVariants[i-1].Location
			current := lineVariants[i].Location
			if current.Start < previous.End || current.Start == previous.Start {
				problems = append(problems, fmt.Sprintf("%s: overlapping variant ranges on %s", label, selectedLine.ID))
			}
		}
		for _, witness := range witnesses {
			if lineIndex >= len(witness.Verses) {
				continue
			}
			reconstructed, err := reconstructWitnessLine(selected, lineVariants, witness.EditionID)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: cannot reconstruct %s from %s variants: %v", label, witness.EditionID, selectedLine.ID, err))
				continue
			}
			if reconstructed != witness.Verses[lineIndex] {
				problems = append(problems, fmt.Sprintf("%s: %s variants reconstruct %s as %q, want witness text %q", label, selectedLine.ID, witness.EditionID, reconstructed, witness.Verses[lineIndex]))
			}
		}
	}
	return problems
}

type selectedLine struct {
	ID   string
	Text string
}

func stanzaLines(work Work, script Script) []selectedLine {
	var result []selectedLine
	for _, section := range work.Sections {
		if section.Kind != "stanza" {
			continue
		}
		for _, line := range section.Lines {
			result = append(result, selectedLine{ID: line.ID, Text: line.Text(script)})
		}
	}
	return result
}

func validVariantsForLine(variants []Variant, lineID string, lineLength int) []Variant {
	result := make([]Variant, 0, len(variants))
	for _, variant := range variants {
		location := variant.Location
		if location.LineID == lineID && location.Start >= 0 && location.End >= location.Start && location.End <= lineLength {
			result = append(result, variant)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Location.Start == result[j].Location.Start {
			return result[i].Location.End < result[j].Location.End
		}
		return result[i].Location.Start < result[j].Location.Start
	})
	return result
}

func reconstructWitnessLine(selected []rune, variants []Variant, editionID string) (string, error) {
	cursor := 0
	var result strings.Builder
	for _, variant := range variants {
		location := variant.Location
		if location.Start < cursor || location.End < location.Start || location.End > len(selected) {
			return "", errors.New("ranges overlap or exceed selected text")
		}
		reading, exists := variantReading(variant, editionID)
		if !exists {
			return "", errors.New("missing witness reading")
		}
		result.WriteString(string(selected[cursor:location.Start]))
		result.WriteString(reading)
		cursor = location.End
	}
	result.WriteString(string(selected[cursor:]))
	return result.String(), nil
}

func variantReading(variant Variant, editionID string) (string, bool) {
	for _, reading := range variant.Readings {
		if reading.EditionID == editionID {
			return reading.Text, true
		}
	}
	return "", false
}

func validateNormalizationRules(rules []NormalizationRule) []string {
	var problems []string
	seenRules := make(map[string]struct{}, len(rules))
	mappings := make(map[string]string, len(rules))
	for i, rule := range rules {
		label := fmt.Sprintf("normalization.rules[%d]", i)
		if strings.TrimSpace(rule.From) == "" || strings.TrimSpace(rule.To) == "" || rule.From == rule.To || !allHan(rule.From) || !allHan(rule.To) {
			problems = append(problems, label+": from/to must be distinct non-empty Han text")
		}
		if strings.TrimSpace(rule.Reason) == "" {
			problems = append(problems, label+": missing reason")
		}
		key := rule.From + "\x00" + rule.To
		if _, duplicate := seenRules[key]; duplicate {
			problems = append(problems, label+": duplicate from/to rule")
		}
		seenRules[key] = struct{}{}
		if existing, conflict := mappings[rule.From]; conflict && existing != rule.To {
			problems = append(problems, fmt.Sprintf("%s: corpus-wide mapping %q conflicts between %q and %q; use a work override", label, rule.From, existing, rule.To))
		} else {
			mappings[rule.From] = rule.To
		}
		seenWorks := make(map[string]struct{}, len(rule.AuditedWorkIDs))
		for _, workID := range rule.AuditedWorkIDs {
			if !validID(workID) {
				problems = append(problems, label+": invalid auditedWorkId "+workID)
			}
			if _, duplicate := seenWorks[workID]; duplicate {
				problems = append(problems, label+": duplicate auditedWorkId "+workID)
			}
			seenWorks[workID] = struct{}{}
		}
	}
	return problems
}

func validateNormalization(label string, work Work, rules []NormalizationRule) []string {
	var problems []string
	overridesByLine := make(map[string][]NormalizationOverride)
	lineByID := make(map[string]Line)
	for _, section := range work.Sections {
		for _, line := range section.Lines {
			lineByID[line.ID] = line
		}
	}
	for i, override := range work.NormalizationOverrides {
		overrideLabel := fmt.Sprintf("%s.normalizationOverrides[%d]", label, i)
		line, exists := lineByID[override.Location.LineID]
		if !exists {
			problems = append(problems, overrideLabel+": unknown lineId "+override.Location.LineID)
			continue
		}
		core, _ := splitTerminalPunctuation(line.Hant)
		runes := []rune(core)
		location := override.Location
		if location.Start < 0 || location.End < location.Start || location.End > len(runes) {
			problems = append(problems, overrideLabel+": invalid zero-based half-open Unicode range")
			continue
		}
		if override.From == override.To || (override.From != "" && !allHan(override.From)) || (override.To != "" && !allHan(override.To)) {
			problems = append(problems, overrideLabel+": from/to must be distinct Han text or one empty side")
		}
		if override.From != string(runes[location.Start:location.End]) {
			problems = append(problems, overrideLabel+": from does not match the traditional line range")
		}
		if strings.TrimSpace(override.Reason) == "" {
			problems = append(problems, overrideLabel+": missing reason")
		}
		overridesByLine[location.LineID] = append(overridesByLine[location.LineID], override)
	}
	for lineID, overrides := range overridesByLine {
		sort.SliceStable(overrides, func(i, j int) bool {
			if overrides[i].Location.Start == overrides[j].Location.Start {
				return overrides[i].Location.End < overrides[j].Location.End
			}
			return overrides[i].Location.Start < overrides[j].Location.Start
		})
		for i := 1; i < len(overrides); i++ {
			previous := overrides[i-1].Location
			current := overrides[i].Location
			if current.Start < previous.End || current.Start == previous.Start {
				problems = append(problems, fmt.Sprintf("%s.normalizationOverrides: overlapping ranges on %s", label, lineID))
			}
		}
		overridesByLine[lineID] = overrides
	}
	if got := applyNormalization(work.Title.Hant, rules); got != work.Title.Hans {
		problems = append(problems, fmt.Sprintf("%s.title: normalization produced %q, want %q", label, got, work.Title.Hans))
	}
	if got := applyNormalization(work.Author.Name.Hant, rules); got != work.Author.Name.Hans {
		problems = append(problems, fmt.Sprintf("%s.author: normalization produced %q, want %q", label, got, work.Author.Name.Hans))
	}
	if work.Tune != nil {
		if got := applyNormalization(work.Tune.Hant, rules); got != work.Tune.Hans {
			problems = append(problems, fmt.Sprintf("%s.tune: normalization produced %q, want %q", label, got, work.Tune.Hans))
		}
	}
	for _, section := range work.Sections {
		for _, line := range section.Lines {
			got, err := normalizeLine(line.Hant, rules, overridesByLine[line.ID])
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s.%s: %v", label, line.ID, err))
				continue
			}
			if got != line.Hans {
				problems = append(problems, fmt.Sprintf("%s.%s: normalization produced %q, want %q", label, line.ID, got, line.Hans))
			}
		}
	}
	return problems
}

func normalizeLine(value string, rules []NormalizationRule, overrides []NormalizationOverride) (string, error) {
	core, punctuation := splitTerminalPunctuation(value)
	runes := []rune(core)
	cursor := 0
	var result strings.Builder
	for _, override := range overrides {
		start := override.Location.Start
		end := override.Location.End
		if start < cursor || end < start || end > len(runes) {
			return "", errors.New("normalization override ranges overlap or exceed the line")
		}
		if override.From != string(runes[start:end]) {
			return "", errors.New("normalization override source does not match the line")
		}
		result.WriteString(applyNormalization(string(runes[cursor:start]), rules))
		result.WriteString(override.To)
		cursor = end
	}
	result.WriteString(applyNormalization(string(runes[cursor:]), rules))
	result.WriteString(punctuation)
	return result.String(), nil
}

func splitTerminalPunctuation(value string) (string, string) {
	runes := []rune(value)
	end := len(runes)
	for end > 0 && unicode.IsPunct(runes[end-1]) {
		end--
	}
	return string(runes[:end]), string(runes[end:])
}

func applyNormalization(value string, rules []NormalizationRule) string {
	sorted := append([]NormalizationRule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i].From) > utf8.RuneCountInString(sorted[j].From)
	})
	var result strings.Builder
	for len(value) > 0 {
		matched := false
		for _, rule := range sorted {
			if strings.HasPrefix(value, rule.From) {
				result.WriteString(rule.To)
				value = strings.TrimPrefix(value, rule.From)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		r, size := utf8.DecodeRuneInString(value)
		result.WriteRune(r)
		value = value[size:]
	}
	return result.String()
}

func workUsesRule(work Work, rule NormalizationRule) bool {
	pairs := [][2]string{{work.Title.Hant, work.Title.Hans}, {work.Author.Name.Hant, work.Author.Name.Hans}}
	if work.Tune != nil {
		pairs = append(pairs, [2]string{work.Tune.Hant, work.Tune.Hans})
	}
	for _, pair := range pairs {
		if strings.Contains(pair[0], rule.From) && applyNormalization(pair[0], []NormalizationRule{rule}) != pair[0] {
			return true
		}
	}
	for _, section := range work.Sections {
		for _, line := range section.Lines {
			var overrides []NormalizationOverride
			for _, override := range work.NormalizationOverrides {
				if override.Location.LineID == line.ID {
					overrides = append(overrides, override)
				}
			}
			withRule, withRuleErr := normalizeLine(line.Hant, []NormalizationRule{rule}, overrides)
			withoutRule, withoutRuleErr := normalizeLine(line.Hant, nil, overrides)
			if withRuleErr == nil && withoutRuleErr == nil && withRule != withoutRule {
				return true
			}
		}
	}
	return false
}

func validID(value string) bool { return idPattern.MatchString(value) }

func validLocalizedText(text LocalizedText, allowPunctuation bool) bool {
	if strings.TrimSpace(text.Hans) == "" || strings.TrimSpace(text.Hant) == "" {
		return false
	}
	validator := allHan
	if allowPunctuation {
		validator = validLiteraryText
	}
	return validator(text.Hans) && validator(text.Hant)
}

func allHan(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return true
}

func validLiteraryText(value string) bool {
	if countContentCharacters(value) == 0 {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.Han, r) || unicode.IsPunct(r) {
			continue
		}
		return false
	}
	return true
}

func validWitnessText(value string) bool {
	return allHan(value)
}

func countContentCharacters(value string) int {
	count := 0
	for _, r := range value {
		if !unicode.IsPunct(r) && !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

func trimTerminalPunctuation(value string) string {
	return strings.TrimRightFunc(value, unicode.IsPunct)
}

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
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
	start, _ := strconv.Atoi(parts[0])
	end, _ := strconv.Atoi(parts[1])
	return start < end
}

func hasWitness(witnesses []Witness, editionID string) bool {
	for _, witness := range witnesses {
		if witness.EditionID == editionID {
			return true
		}
	}
	return false
}

func validatePosition(label, status string, position *int) []string {
	switch status {
	case "confirmed":
		if position == nil || *position <= 0 {
			return []string{label + ": confirmed position must be positive"}
		}
	case "pending":
		if position != nil {
			return []string{label + ": pending position must be omitted"}
		}
	default:
		return []string{label + ": positionStatus must be confirmed or pending"}
	}
	return nil
}

func collectionHasMember(collection Collection, workID string, membership WorkCollection) bool {
	for _, member := range collection.Members {
		if member.WorkID == workID && member.PositionStatus == membership.PositionStatus && equalIntPointers(member.Position, membership.Position) {
			return true
		}
	}
	return false
}

func hasMembership(work Work, collectionID string, member CollectionMember) bool {
	for _, membership := range work.Collections {
		if membership.ID == collectionID && membership.PositionStatus == member.PositionStatus && equalIntPointers(membership.Position, member.Position) {
			return true
		}
	}
	return false
}

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func workContentKey(work Work, script Script) string {
	var lines []string
	for _, section := range work.Sections {
		for _, line := range section.Lines {
			lines = append(lines, line.Text(script))
		}
	}
	return strings.Join(lines, "\x00")
}
