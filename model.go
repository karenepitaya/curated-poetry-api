package poetryapi

// Script selects the writing system returned to API consumers.
type Script string

const (
	ScriptHans Script = "hans"
	ScriptHant Script = "hant"

	DynastyTang = "tang"
	DynastySong = "song"

	GenreShi = "shi"
	GenreCi  = "ci"

	FormGushi = "gushi"
	FormLushi = "lushi"
	FormJueju = "jueju"
	FormCi    = "ci"

	MeterFive  = "5"
	MeterSeven = "7"
	MeterMixed = "mixed"

	EvidencePrimaryScanReviewed = "primary-scan-reviewed"

	// Deprecated legacy values kept until /api/poems/random is removed.
	TypeFiveCharacter  = "五言绝句"
	TypeSevenCharacter = "七言绝句"
)

// LocalizedText stores the curated simplified and traditional renderings.
type LocalizedText struct {
	Hans string `json:"hans"`
	Hant string `json:"hant"`
}

func (t LocalizedText) For(script Script) string {
	if script == ScriptHant {
		return t.Hant
	}
	return t.Hans
}

type Author struct {
	Name              LocalizedText `json:"name"`
	AttributionStatus string        `json:"attributionStatus"`
	AttributionNote   string        `json:"attributionNote,omitempty"`
}

type Line struct {
	ID   string `json:"id"`
	Hans string `json:"hans"`
	Hant string `json:"hant"`
}

func (l Line) Text(script Script) string {
	if script == ScriptHant {
		return l.Hant
	}
	return l.Hans
}

type Section struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Lines []Line `json:"lines"`
}

type WorkCollection struct {
	ID             string `json:"id"`
	Position       *int   `json:"position,omitempty"`
	PositionStatus string `json:"positionStatus"`
}

// VariantLocation uses zero-based, half-open Unicode code-point offsets in the
// traditional line text, excluding terminal punctuation. Start may equal End
// to anchor an omission recorded by a witness.
type VariantLocation struct {
	LineID string `json:"lineId"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
}

type VariantReading struct {
	EditionID string `json:"editionId"`
	Text      string `json:"text"`
}

type Variant struct {
	Location  VariantLocation  `json:"location"`
	Readings  []VariantReading `json:"readings"`
	Chosen    string           `json:"chosen"`
	Rationale string           `json:"rationale"`
}

type Witness struct {
	EditionID    string   `json:"editionId"`
	ScanPage     string   `json:"scanPage"`
	PrintedFolio string   `json:"printedFolio"`
	Verses       []string `json:"verses"`
}

type WorkEvidence struct {
	Level        string    `json:"level"`
	Status       string    `json:"status"`
	Witnesses    []Witness `json:"witnesses"`
	Variants     []Variant `json:"variants"`
	ReviewedAt   string    `json:"reviewedAt"`
	ReviewMethod string    `json:"reviewMethod"`
}

// Work is the canonical, variable-length corpus record.
type Work struct {
	ID          string           `json:"id"`
	Title       LocalizedText    `json:"title"`
	Author      Author           `json:"author"`
	Dynasty     string           `json:"dynasty"`
	Genre       string           `json:"genre"`
	Form        string           `json:"form"`
	Meter       string           `json:"meter"`
	Tune        *LocalizedText   `json:"tune,omitempty"`
	Sections    []Section        `json:"sections"`
	Collections []WorkCollection `json:"collections"`
	// NormalizationOverrides records context-sensitive Hant-to-Hans edits that
	// cannot be represented safely by a corpus-wide character rule.
	NormalizationOverrides []NormalizationOverride `json:"normalizationOverrides,omitempty"`
	Evidence               WorkEvidence            `json:"evidence"`
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

type CollectionMember struct {
	WorkID         string `json:"workId"`
	Position       *int   `json:"position,omitempty"`
	PositionStatus string `json:"positionStatus"`
}

type Collection struct {
	ID               string             `json:"id"`
	Title            LocalizedText      `json:"title"`
	Status           string             `json:"status"`
	PrimaryEditionID string             `json:"primaryEditionId"`
	ExpectedCount    int                `json:"expectedCount,omitempty"`
	Members          []CollectionMember `json:"members"`
}

// NormalizationRule applies corpus-wide. AuditedWorkIDs preserves the legacy
// migration audit trail; it does not limit where the rule is applied.
type NormalizationRule struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Reason         string   `json:"reason"`
	AuditedWorkIDs []string `json:"auditedWorkIds,omitempty"`
}

type NormalizationOverride struct {
	Location VariantLocation `json:"location"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Reason   string          `json:"reason"`
}

// Poem is the deprecated four-line view used by the compatibility endpoint.
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

type Query struct {
	Collection string
	Dynasty    string
	Genre      string
	Form       string
	Meter      string
	MaxChars   int
	Script     Script
}

type CorpusStats struct {
	Works          int
	ByDynasty      map[string]int
	CorpusRevision string
}
