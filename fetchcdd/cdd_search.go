package fetchcdd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const baseURL = "https://cdd.iec.ch/cdd/"
const dataSpecURL = "http://admin-shell.io/DataSpecificationTemplates/DataSpecificationIEC61360/3/0"
const dataFilename = "data.json"

// ---------- input / URL helpers ----------

func cleanInput(irdi string) (string, error) {
	irdi = strings.TrimSpace(irdi)
	if len(irdi) < 4 {
		return "", fmt.Errorf("input too short to remove the last 4 characters")
	}
	trimmed := strings.TrimSpace(irdi[:len(irdi)-4])
	trimmed = strings.ReplaceAll(trimmed, "/", "-")
	trimmed = strings.ReplaceAll(trimmed, "#", "%23")
	return trimmed, nil
}

func extractNumber(irdi string) (string, bool) {
	re := regexp.MustCompile(`///([\w]+)#`)
	m := re.FindStringSubmatch(irdi)
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}

func buildURL(number, cleaned string) string {
	if strings.EqualFold(number, "ICS") {
		return fmt.Sprintf("%sisoics/isoics.nsf/TU0/%s", baseURL, cleaned)
	}
	if number == "63213" {
		return fmt.Sprintf("%siectc85/iec63213.nsf/TU0/%s", baseURL, cleaned)
	}
	if strings.Contains(number, "_") {
		prefix := "iec" + strings.ReplaceAll(number, "_", "-")
		return fmt.Sprintf("%s%s/%s.nsf/TU0/%s", baseURL, prefix, prefix, cleaned)
	}
	prefix := "iec" + number
	return fmt.Sprintf("%s%s/%s.nsf/TU0/%s", baseURL, prefix, prefix, cleaned)
}

// ---------- HTTP + HTML (no saving) ----------

func fetchEnglishSection(url string) (*html.Node, bool) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf(" Error creating request: %v\n", err)
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf(" Error fetching URL: %v\n", err)
		return nil, false
	}
	defer resp.Body.Close()

	fmt.Printf(" HTTP status code: %d\n", resp.StatusCode)

	doc, err := html.Parse(resp.Body)
	if err != nil {
		fmt.Printf(" Error parsing HTML: %v\n", err)
		return nil, false
	}

	target := findElementByID(doc, "onglet1") // English tab
	if target == nil {
		fmt.Println(" No English section found.")
		return nil, false
	}

	return target, true
}

func findElementByID(n *html.Node, id string) *html.Node {
	var dfs func(*html.Node) *html.Node
	dfs = func(node *html.Node) *html.Node {
		if node.Type == html.ElementNode {
			for _, a := range node.Attr {
				if a.Key == "id" && a.Val == id {
					return node
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if found := dfs(c); found != nil {
				return found
			}
		}
		return nil
	}
	return dfs(n)
}

// ---------- field extraction ----------

func normalizeSpaces(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(strings.ReplaceAll(s, "\u00A0", " "))
	return strings.Join(fields, " ")
}

func normalizeKey(s string) string {
	s = normalizeSpaces(s)
	s = strings.TrimSuffix(s, ":") // strip trailing colon used by CDD labels
	return strings.ToLower(s)
}

func extractFields(n *html.Node) map[string]string {
	fields := map[string]string{}

	var walk func(*html.Node)
	walk = func(x *html.Node) {
		// tables: TR with TH/TD
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "tr") {
			var cells []string
			for c := x.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (strings.EqualFold(c.Data, "td") || strings.EqualFold(c.Data, "th")) {
					cells = append(cells, normalizeSpaces(innerText(c)))
				}
			}
			if len(cells) >= 2 {
				key := normalizeKey(cells[0])
				fields[key] = cells[1]
			}
		}
		// definition lists: DT → DD
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "dt") {
			key := normalizeKey(innerText(x))
			if dd := nextElementSibling(x, "dd"); dd != nil {
				fields[key] = normalizeSpaces(innerText(dd))
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return fields
}

func innerText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return normalizeSpaces(b.String())
}

func nextElementSibling(n *html.Node, tag string) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && strings.EqualFold(s.Data, tag) {
			return s
		}
	}
	return nil
}

// ---------- AAS ConceptDescription ----------

type LangString struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

type ValueReferencePair struct {
	Value   string `json:"value"`
	ValueId string `json:"valueId,omitempty"`
}

type DataSpecificationIec61360 struct {
	ModelType     string               `json:"modelType"`
	DataType      string               `json:"dataType,omitempty"`
	Definition    []LangString         `json:"definition"`
	PreferredName []LangString         `json:"preferredName"`
	Unit          string               `json:"unit,omitempty"`
	UnitId        string               `json:"unitId,omitempty"`
	Symbol        string               `json:"symbol,omitempty"`
	ValueList     []ValueReferencePair `json:"valueList,omitempty"`
	LevelType     string               `json:"levelType,omitempty"`
	SourceOfDef   string               `json:"sourceOfDefinition,omitempty"`
	ValueFormat   string               `json:"valueFormat,omitempty"`
}

type Key struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Reference struct {
	Keys []Key  `json:"keys"`
	Type string `json:"type"`
}

type EmbeddedDataSpecification struct {
	DataSpecification        Reference                 `json:"dataSpecification"`
	DataSpecificationContent DataSpecificationIec61360 `json:"dataSpecificationContent"`
}

type ConceptDescription struct {
	ModelType                  string                      `json:"modelType"`
	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications"`
	Id                         string                      `json:"id"`
	IdShort                    string                      `json:"idShort"`
}

// ---------- data.json structure (paging_metadata + result) ----------

type DataFile struct {
	PagingMetadata map[string]any       `json:"paging_metadata"`
	Result         []ConceptDescription `json:"result"`
}

// readDataFile returns an initialized DataFile (empty skeleton if file missing/empty)
func readDataFile(filename string) (DataFile, error) {
	df := DataFile{PagingMetadata: map[string]any{}, Result: []ConceptDescription{}}

	b, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return df, nil
		}
		return df, fmt.Errorf("failed to read %s: %w", filename, err)
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return df, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &df); err != nil {
		return df, fmt.Errorf("failed to parse %s: %w", filename, err)
	}
	// ensure non-nil fields
	if df.PagingMetadata == nil {
		df.PagingMetadata = map[string]any{}
	}
	if df.Result == nil {
		df.Result = []ConceptDescription{}
	}
	return df, nil
}

func writeDataFileAtomic(filename string, df DataFile) error {
	out, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		return err
	}
	tmp := filename + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("failed to write temp file %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, filename); err != nil {
		return fmt.Errorf("failed to replace %s: %w", filename, err)
	}
	return nil
}

func idExistsInDataFile(id, filename string) (bool, error) {
	df, err := readDataFile(filename)
	if err != nil {
		return false, err
	}
	for _, item := range df.Result {
		if item.Id == id {
			return true, nil
		}
	}
	return false, nil
}

func appendConceptDescriptionToDataFile(cd ConceptDescription, filename string) error {
	df, err := readDataFile(filename)
	if err != nil {
		return err
	}
	df.Result = append(df.Result, cd)
	return writeDataFileAtomic(filename, df)
}

// ---------- strict mapping ----------

func buildConceptDescriptionStrict(fields map[string]string, irdi string) (ConceptDescription, error) {
	labelMap := map[string][]string{
		"preferredName": {"preferred name", "english name", "preferred name (en)"},
		"shortName":     {"short name"},
		"definition":    {"definition"},
		"unit":          {"unit", "unit (symbol)", "uom"},
		"unitId":        {"unit irdi", "unit id", "unit (irdi)"},
		"symbol":        {"symbol"},
		"dataType":      {"data type", "datatype", "iec 61360 data type"},
		"valueFormat":   {"format", "value format"},
		"levelType":     {"level type"},
		"sourceOfDef":   {"source of definition", "definition source", "source"},
	}

	get := func(keys []string) string {
		for _, k := range keys {
			if v, ok := fields[k]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}

	pref := get(labelMap["preferredName"])
	if pref == "" {
		var have []string
		for k := range fields {
			have = append(have, k)
		}
		return ConceptDescription{}, fmt.Errorf("missing required field: preferredName (available keys: %v)", have)
	}
	defn := get(labelMap["definition"])
	if defn == "" {
		return ConceptDescription{}, fmt.Errorf("missing required field: definition")
	}

	dtRaw := strings.TrimSpace(get(labelMap["dataType"]))
	dt, err := mapDataTypeStrict(dtRaw)
	if err != nil {
		return ConceptDescription{}, err
	}

	ds := DataSpecificationIec61360{
		ModelType:     "DataSpecificationIec61360",
		DataType:      dt,
		Definition:    []LangString{{Language: "en", Text: defn}},
		PreferredName: []LangString{{Language: "en", Text: pref}},
		Unit:          get(labelMap["unit"]),
		UnitId:        get(labelMap["unitId"]),
		Symbol:        get(labelMap["symbol"]),
		LevelType:     get(labelMap["levelType"]),
		SourceOfDef:   get(labelMap["sourceOfDef"]),
		ValueFormat:   strings.TrimSpace(get(labelMap["valueFormat"])),
	}
	ds.ValueList = extractValues(fields)

	eds := EmbeddedDataSpecification{
		DataSpecification: Reference{
			Keys: []Key{{Type: "GlobalReference", Value: dataSpecURL}},
			Type: "ExternalReference",
		},
		DataSpecificationContent: ds,
	}

	cd := ConceptDescription{
		ModelType:                  "ConceptDescription",
		EmbeddedDataSpecifications: []EmbeddedDataSpecification{eds},
		Id:                         irdi,
		IdShort:                    pref,
	}
	return cd, nil
}

func mapDataTypeStrict(dt string) (string, error) {
	if dt == "" {
		return "", nil
	}
	allowed := map[string]struct{}{
		"STRING":           {},
		"BOOLEAN":          {},
		"DATE":             {},
		"TIME":             {},
		"REAL_MEASURE":     {},
		"INTEGER_MEASURE":  {},
		"RATIONAL":         {},
		"RATIONAL_MEASURE": {},
		"COUNT":            {},
		"BLOB":             {},
		"FILE":             {},
		"IRI":              {},
		"IRDI":             {},
		"LANGSTRING":       {},
		"BOOLEAN_MEASURE":  {},
		"TIME_OF_DAY":      {},
	}
	u := strings.ToUpper(dt)
	if _, ok := allowed[u]; !ok {
		return "", fmt.Errorf("invalid IEC61360 data type: %q (heuristics not allowed)", dt)
	}
	return u, nil
}

func extractValues(fields map[string]string) []ValueReferencePair {
	var pairs []ValueReferencePair
	for k, v := range fields {
		if strings.Contains(k, "value list") || strings.Contains(k, "permitted values") || strings.Contains(k, "enumeration") {
			for _, t := range splitList(v) {
				if t == "" {
					continue
				}
				val, id := splitValueAndIRDI(t)
				pairs = append(pairs, ValueReferencePair{Value: val, ValueId: id})
			}
		}
	}
	return pairs
}

func splitList(s string) []string {
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, ";", "\n")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' })
	var out []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitValueAndIRDI(s string) (val, id string) {
	if i := strings.LastIndex(s, "("); i > 0 && strings.HasSuffix(s, ")") {
		val = strings.TrimSpace(s[:i])
		id = strings.TrimSpace(strings.TrimSuffix(s[i+1:], ")"))
		return
	}
	return strings.TrimSpace(s), ""
}

func GetIRDIfromCS(irdi string) error {
	fmt.Printf("fetching IRDI %s:\n", irdi)
	userInput := strings.TrimSpace(irdi)

	exists, err := idExistsInDataFile(userInput, dataFilename)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", dataFilename, err)
	}
	if exists {
		fmt.Printf("Entry with id %s already exists in %s — skipping.\n", userInput, dataFilename)
		return nil
	}

	cleaned, err := cleanInput(userInput)
	if err != nil {
		return err
	}

	number, ok := extractNumber(userInput)
	if !ok {
		return fmt.Errorf("no valid number found between '///' and '#'")
	}

	fullURL := buildURL(number, cleaned)
	fmt.Printf("\n Fetching URL:\n%s\n\n", fullURL)

	node, ok := fetchEnglishSection(fullURL)
	if !ok || node == nil {
		return fmt.Errorf("failed to fetch English section")
	}

	fields := extractFields(node)

	cd, err := buildConceptDescriptionStrict(fields, userInput)
	if err != nil {
		return fmt.Errorf("error building ConceptDescription: %v", err)
	}

	if err := appendConceptDescriptionToDataFile(cd, dataFilename); err != nil {
		return fmt.Errorf("error updating %s: %v", dataFilename, err)
	}

	fmt.Printf("Appended ConceptDescription to %s (id: %s)\n", dataFilename, cd.Id)
	return nil
}
