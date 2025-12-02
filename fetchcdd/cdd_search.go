package fetchcdd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	aasjson "github.com/aas-core-works/aas-core3.0-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.0-golang/types"
)

const BASEURL_CDD = "https://cdd.iec.ch/cdd/"
const DATA_SPECIFICATION_URL = "http://admin-shell.io/DataSpecificationTemplates/DataSpecificationIEC61360/3/0"
const DATAFILE_NAME = "concept-descriptions-database.json"

// concept-descriptions-database.json
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
		return fmt.Sprintf("%sisoics/isoics.nsf/TU0/%s", BASEURL_CDD, cleaned)
	}
	if number == "63213" {
		return fmt.Sprintf("%siectc85/iec63213.nsf/TU0/%s", BASEURL_CDD, cleaned)
	}
	if strings.Contains(number, "_") {
		cleanedNumber := regexp.MustCompile(`_\d`).ReplaceAllString(number, "")
		prefix := "iec" + strings.ReplaceAll(cleanedNumber, "_", "")
		return fmt.Sprintf("%s%s/%s.nsf/TU0/%s", BASEURL_CDD, prefix, prefix, cleaned)
	}
	prefix := "iec" + number
	return fmt.Sprintf("%s%s/%s.nsf/TU0/%s", BASEURL_CDD, prefix, prefix, cleaned)
}

func fetchEnglishSection(url string) (*html.Node, bool) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching URL: %v\n", err)
		return nil, false
	}
	defer resp.Body.Close()

	fmt.Printf("HTTP status code: %d\n", resp.StatusCode)

	doc, err := html.Parse(resp.Body)
	if err != nil {
		fmt.Printf("Error parsing HTML: %v\n", err)
		return nil, false
	}

	target := findElementByID(doc, "onglet1")
	if target == nil {
		fmt.Println("No English section found.")
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

func normalizeSpaces(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(strings.ReplaceAll(s, "\u00A0", " "))
	return strings.Join(fields, " ")
}

func normalizeKey(s string) string {
	s = normalizeSpaces(s)
	s = strings.TrimSuffix(s, ":")
	return strings.ToLower(s)
}

func extractFields(n *html.Node) map[string]string {
	fields := map[string]string{}

	var walk func(*html.Node)
	walk = func(x *html.Node) {
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

type DataFile struct {
	PagingMetadata map[string]any                 `json:"paging_metadata"`
	Result         []aastypes.IConceptDescription `json:"-"`
	RawResult      []json.RawMessage              `json:"result,omitempty"`
}

func marshalDataFile(df DataFile) ([]byte, error) {
	df.RawResult = nil

	for _, cd := range df.Result {
		if cd == nil {
			continue
		}

		jsonable, err := aasjson.ToJsonable(cd)
		if err != nil {
			return nil, fmt.Errorf("failed to convert ConceptDescription to jsonable: %w", err)
		}

		b, err := json.Marshal(jsonable)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jsonable ConceptDescription: %w", err)
		}

		df.RawResult = append(df.RawResult, b)
	}

	return json.MarshalIndent(df, "", "  ")
}

func unmarshalDataFile(data []byte) (DataFile, error) {
	var df DataFile

	if err := json.Unmarshal(data, &df); err != nil {
		return df, fmt.Errorf("failed to unmarshal data file wrapper: %w", err)
	}

	if df.PagingMetadata == nil {
		df.PagingMetadata = map[string]any{}
	}

	df.Result = nil

	for idx, raw := range df.RawResult {
		if len(raw) == 0 {
			continue
		}

		var jsonable any
		if err := json.Unmarshal(raw, &jsonable); err != nil {
			return df, fmt.Errorf("failed to unmarshal result[%d] raw JSON: %w", idx, err)
		}

		cd, err := aasjson.ConceptDescriptionFromJsonable(jsonable)
		if err != nil {
			return df, fmt.Errorf("failed to convert jsonable to ConceptDescription at index %d: %w", idx, err)
		}

		df.Result = append(df.Result, cd)
	}

	return df, nil
}

func ReadDataFile(filename string) (DataFile, error) {
	df := DataFile{
		PagingMetadata: map[string]any{},
		Result:         []aastypes.IConceptDescription{},
	}

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

	parsed, err := unmarshalDataFile([]byte(trimmed))
	if err != nil {
		return df, fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	if parsed.PagingMetadata == nil {
		parsed.PagingMetadata = map[string]any{}
	}
	if parsed.Result == nil {
		parsed.Result = []aastypes.IConceptDescription{}
	}

	return parsed, nil
}

func WriteDataFileAtomic(filename string, df DataFile) error {
	out, err := marshalDataFile(df)
	if err != nil {
		return fmt.Errorf("failed to marshal data file: %w", err)
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

func IdExistsInDataFile(id, filename string) (bool, error) {
	df, err := ReadDataFile(filename)
	if err != nil {
		return false, err
	}
	for _, item := range df.Result {
		if item.ID() == id {
			return true, nil
		}
	}
	return false, nil
}

func appendConceptDescriptionToDataFile(cd aastypes.IConceptDescription, filename string) error {
	df, err := ReadDataFile(filename)
	if err != nil {
		return err
	}
	df.Result = append(df.Result, cd)
	return WriteDataFileAtomic(filename, df)
}

func buildConceptDescriptionStrict(fields map[string]string, irdi string) (aastypes.IConceptDescription, error) {
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
		return nil, fmt.Errorf("missing required field: preferredName (available keys: %v)", have)
	}
	defn := get(labelMap["definition"])
	if defn == "" {
		return nil, fmt.Errorf("missing required field: definition")
	}

	dtRaw := strings.TrimSpace(get(labelMap["dataType"]))
	dt, err := mapDataTypeStrict(dtRaw)
	if err != nil {
		return nil, err
	}

	preferredName := []aastypes.ILangStringPreferredNameTypeIEC61360{
		aastypes.NewLangStringPreferredNameTypeIEC61360("en", pref),
	}
	definition := []aastypes.ILangStringDefinitionTypeIEC61360{
		aastypes.NewLangStringDefinitionTypeIEC61360("en", defn),
	}

	valueList := extractValues(fields)

	ds := aastypes.NewDataSpecificationIEC61360(preferredName)
	if dt != nil {
		ds.SetDataType(dt)
	}
	ds.SetDefinition(definition)

	if u := get(labelMap["unit"]); u != "" {
		ds.SetUnit(&u)
	}
	if s := get(labelMap["symbol"]); s != "" {
		ds.SetSymbol(&s)
	}
	if vf := get(labelMap["valueFormat"]); vf != "" {
		ds.SetValueFormat(&vf)
	}
	if src := get(labelMap["sourceOfDef"]); src != "" {
		ds.SetSourceOfDefinition(&src)
	}
	if valueList != nil {
		ds.SetValueList(valueList)
	}

	key := aastypes.NewKey(aastypes.KeyTypesGlobalReference, DATA_SPECIFICATION_URL)
	ref := aastypes.NewReference(
		aastypes.ReferenceTypesExternalReference,
		[]aastypes.IKey{key},
	)

	eds := aastypes.NewEmbeddedDataSpecification(ref, ds)

	cd := aastypes.NewConceptDescription(irdi)
	cd.SetIDShort(&pref)
	cd.SetEmbeddedDataSpecifications([]aastypes.IEmbeddedDataSpecification{eds})

	return cd, nil
}

func mapDataTypeStrict(dtRaw string) (*aastypes.DataTypeIEC61360, error) {
	if dtRaw == "" {
		return nil, nil
	}
	u := strings.ToUpper(strings.TrimSpace(dtRaw))

	var v aastypes.DataTypeIEC61360

	switch u {
	case "STRING":
		v = aastypes.DataTypeIEC61360String
	case "STRING_TRANSLATABLE":
		v = aastypes.DataTypeIEC61360StringTranslatable
	case "BOOLEAN":
		v = aastypes.DataTypeIEC61360Boolean
	case "REAL_MEASURE":
		v = aastypes.DataTypeIEC61360RealMeasure
	case "REAL_COUNT":
		v = aastypes.DataTypeIEC61360RealCount
	case "REAL_CURRENCY":
		v = aastypes.DataTypeIEC61360RealCurrency
	case "INTEGER_MEASURE":
		v = aastypes.DataTypeIEC61360IntegerMeasure
	case "INTEGER_COUNT":
		v = aastypes.DataTypeIEC61360IntegerCount
	case "INTEGER_CURRENCY":
		v = aastypes.DataTypeIEC61360IntegerCurrency
	case "RATIONAL":
		v = aastypes.DataTypeIEC61360Rational
	case "RATIONAL_MEASURE":
		v = aastypes.DataTypeIEC61360RationalMeasure
	case "DATE":
		v = aastypes.DataTypeIEC61360Date
	case "TIME":
		v = aastypes.DataTypeIEC61360Time
	case "TIMESTAMP":
		v = aastypes.DataTypeIEC61360Timestamp
	case "IRI":
		v = aastypes.DataTypeIEC61360IRI
	case "IRDI":
		v = aastypes.DataTypeIEC61360IRDI
	case "FILE":
		v = aastypes.DataTypeIEC61360File
	case "BLOB":
		v = aastypes.DataTypeIEC61360Blob
	case "HTML":
		v = aastypes.DataTypeIEC61360HTML
	default:
		return nil, fmt.Errorf("invalid IEC61360 data type: %q", dtRaw)
	}

	return &v, nil
}

func extractValues(fields map[string]string) aastypes.IValueList {
	var pairs []aastypes.IValueReferencePair

	for k, v := range fields {
		if strings.Contains(k, "value list") ||
			strings.Contains(k, "permitted values") ||
			strings.Contains(k, "enumeration") {

			for _, t := range splitList(v) {
				if t == "" {
					continue
				}
				val, id := splitValueAndIRDI(t)

				var ref aastypes.IReference
				if id != "" {
					key := aastypes.NewKey(aastypes.KeyTypesGlobalReference, id)
					ref = aastypes.NewReference(
						aastypes.ReferenceTypesExternalReference,
						[]aastypes.IKey{key},
					)
				}

				vrp := aastypes.NewValueReferencePair(val, ref)
				pairs = append(pairs, vrp)
			}
		}
	}

	if len(pairs) == 0 {
		return nil
	}
	return aastypes.NewValueList(pairs)
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

// main function
func GetIRDIfromCS(irdi string, filename string) error {
	// default: concept-store directory
	if strings.TrimSpace(filename) == "" {
		filename = DATAFILE_NAME
	}

	fmt.Printf("fetching IRDI %s:\n", irdi)
	userInput := strings.TrimSpace(irdi)

	exists, err := IdExistsInDataFile(userInput, filename)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", filename, err)
	}
	if exists {
		fmt.Printf("Entry with id %s already exists in %s — skipping.\n", userInput, filename)
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

	if err := appendConceptDescriptionToDataFile(cd, filename); err != nil {
		return fmt.Errorf("error updating %s: %v", filename, err)
	}

	fmt.Printf("Appended ConceptDescription to %s (id: %s)\n", filename, cd.ID())
	return nil
}
