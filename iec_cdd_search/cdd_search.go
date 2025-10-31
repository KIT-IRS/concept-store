package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const baseURL = "https://cdd.iec.ch/cdd/"
const dsNS = "http://admin-shell.io/DataSpecificationTemplates/DataSpecificationIec61360/3/0"

func cleanInput(irdi string) (string, error) {
	irdi = strings.TrimSpace(irdi)
	if len(irdi) < 4 {
		return "", fmt.Errorf("Eingabe ist zu kurz, um die letzten 4 Zeichen zu entfernen")
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

// ---- HTTP + HTML speichern & Node zurückgeben ----
func fetchEnglishSection(url, filename string) (*html.Node, bool) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf(" Fehler beim Erstellen der Anfrage: %v\n", err)
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf(" Fehler beim Abrufen der URL: %v\n", err)
		return nil, false
	}
	defer resp.Body.Close()

	fmt.Printf(" Statuscode: %d\n", resp.StatusCode)

	doc, err := html.Parse(resp.Body)
	if err != nil {
		fmt.Printf(" Fehler beim Parsen des HTML: %v\n", err)
		return nil, false
	}

	target := findElementByID(doc, "onglet1")
	if target == nil {
		fmt.Println(" Kein englischer Abschnitt gefunden.")
		return nil, false
	}

	f, err := os.Create(filename)
	if err != nil {
		fmt.Printf(" Fehler beim Speichern der Datei: %v\n", err)
		return nil, false
	}
	defer f.Close()

	if err := renderNode(f, target); err != nil {
		fmt.Printf(" Fehler beim Schreiben der Datei: %v\n", err)
		return nil, false
	}

	fmt.Printf(" Gespeichert in: %s\n", filename)
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

func renderNode(w io.Writer, n *html.Node) error {
	_, _ = io.WriteString(w, "<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body>")
	if err := html.Render(w, n); err != nil {
		return err
	}
	_, _ = io.WriteString(w, "</body></html>")
	return nil
}

// ---------- IEC 61360 (AAS) XML ----------

type LangString struct {
	Language string `xml:"lang,attr,omitempty"` // xml:lang wäre sauberer mit Namespace, hier schlicht "lang"
	Text     string `xml:",chardata"`
}

type ValueReferencePair struct {
	Value   string `xml:"value"`
	ValueId string `xml:"valueId,omitempty"`
}

type DataSpecificationIec61360 struct {
	XMLName       xml.Name             `xml:"DataSpecificationIec61360"`
	XMLNS         string               `xml:"xmlns,attr"`
	PreferredName LangString           `xml:"preferredName"`
	ShortName     *LangString          `xml:"shortName,omitempty"`
	Definition    LangString           `xml:"definition"`
	DataType      string               `xml:"dataType,omitempty"`
	ValueFormat   string               `xml:"valueFormat,omitempty"`
	Unit          string               `xml:"unit,omitempty"`
	UnitId        string               `xml:"unitId,omitempty"`
	Symbol        string               `xml:"symbol,omitempty"`
	ValueList     []ValueReferencePair `xml:"valueList>valueReferencePair,omitempty"`
	LevelType     string               `xml:"levelType,omitempty"`
	SourceOfDef   string               `xml:"sourceOfDefinition,omitempty"`
}

// Heuristik: extrahiere Label→Wert-Paare aus Tabellen, DLs und "Label: Wert"-Zeilen
func extractFields(n *html.Node) map[string]string {
	fields := map[string]string{}

	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Join(strings.Fields(s), " ")
		return s
	}

	// Sammle alle Textblätter (für einfache "Label: Wert"-Muster)
	var texts []string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			t := normalize(x.Data)
			if t != "" {
				texts = append(texts, t)
			}
		}
		// Tabelle TR->(TH/TD)
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "tr") {
			var cells []string
			for c := x.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (strings.EqualFold(c.Data, "td") || strings.EqualFold(c.Data, "th")) {
					cells = append(cells, normalize(innerText(c)))
				}
			}
			if len(cells) >= 2 {
				key := strings.ToLower(cells[0])
				fields[key] = cells[1]
			}
		}
		// DL -> DT/DD
		if x.Type == html.ElementNode && strings.EqualFold(x.Data, "dt") {
			key := strings.ToLower(normalize(innerText(x)))
			if dd := nextElementSibling(x, "dd"); dd != nil {
				fields[key] = normalize(innerText(dd))
			}
		}

		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	// Fallback: „Label: Wert“-Paare in linearer Reihenfolge
	for i := 0; i < len(texts)-1; i++ {
		if strings.HasSuffix(texts[i], ":") && !strings.Contains(texts[i+1], ":") {
			key := strings.ToLower(strings.TrimSuffix(texts[i], ":"))
			if _, exists := fields[key]; !exists {
				fields[key] = texts[i+1]
			}
		}
	}

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
	return strings.Join(strings.Fields(b.String()), " ")
}

func nextElementSibling(n *html.Node, tag string) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && strings.EqualFold(s.Data, tag) {
			return s
		}
	}
	return nil
}

// Mapping CDD-Labels → IEC61360 Felder
func buildIEC61360(fields map[string]string, irdi string) DataSpecificationIec61360 {
	// Mögliche Labelvarianten (je nach Seite)
	labelMap := map[string][]string{
		"preferredName": {"preferred name", "name", "english name"},
		"shortName":     {"short name"},
		"definition":    {"definition"},
		"unit":          {"unit", "unit (symbol)", "uom"},
		"unitId":        {"unit irdi", "unit id", "unit (irdi)"},
		"symbol":        {"symbol"},
		"dataType":      {"data type", "datatype", "iec 61360 data type"},
		"valueFormat":   {"format", "value format"},
		"levelType":     {"level type"},
		"sourceOfDef":   {"source of definition", "source"},
	}

	get := func(keys []string) string {
		for _, k := range keys {
			if v, ok := fields[k]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}

	// preferredName & definition sind in IEC61360 Pflicht für PROPERTY (vgl. Tabelle in Spec)
	pref := get(labelMap["preferredName"])
	if pref == "" {
		// Fallback: versuche „designation“ o.ä.
		for k, v := range fields {
			if strings.Contains(k, "designation") || strings.Contains(k, "title") {
				pref = v
				break
			}
		}
		if pref == "" {
			pref = irdi // letzter Notnagel, damit XML formal vollständig ist
		}
	}
	defn := get(labelMap["definition"])
	if defn == "" {
		// Fallback: „note“/„remark“
		for k, v := range fields {
			if strings.Contains(k, "definition") || strings.Contains(k, "note") || strings.Contains(k, "remark") {
				defn = v
				break
			}
		}
		if defn == "" {
			defn = "N/A"
		}
	}

	ds := DataSpecificationIec61360{
		XMLNS:         dsNS,
		PreferredName: LangString{Language: "en", Text: pref},
		Definition:    LangString{Language: "en", Text: defn},
		DataType:      mapDataType(get(labelMap["dataType"])),
		ValueFormat:   normalizeFormat(get(labelMap["valueFormat"])),
		Unit:          get(labelMap["unit"]),
		UnitId:        get(labelMap["unitId"]),
		Symbol:        get(labelMap["symbol"]),
		LevelType:     get(labelMap["levelType"]),
		SourceOfDef:   get(labelMap["sourceOfDef"]),
	}

	if s := get(labelMap["shortName"]); s != "" {
		ds.ShortName = &LangString{Language: "en", Text: s}
	}

	// ValueList (sofern im HTML vorhanden, häufig als Aufzählung)
	ds.ValueList = extractValues(fields)

	return ds
}

func mapDataType(dt string) string {
	// Normalisiere gängige Schreibweisen auf die IEC61360-Typen lt. AAS-Mapping (z.B. STRING, BOOLEAN, REAL_MEASURE, ...).
	u := strings.ToUpper(strings.TrimSpace(dt))
	switch u {
	case "STRING", "BOOLEAN", "DATE", "TIME", "REAL_MEASURE", "INTEGER_MEASURE", "RATIONAL", "RATIONAL_MEASURE",
		"COUNT", "BLOB", "FILE", "IRI", "IRDI", "LANGSTRING", "BOOLEAN_MEASURE", "TIME_OF_DAY":
		return u
	}
	// Heuristik:
	if strings.Contains(u, "REAL") || strings.Contains(u, "FLOAT") || strings.Contains(u, "DOUBLE") {
		return "REAL_MEASURE"
	}
	if strings.Contains(u, "INT") {
		return "INTEGER_MEASURE"
	}
	if strings.Contains(u, "BOOL") {
		return "BOOLEAN"
	}
	if strings.Contains(u, "STRING") || strings.Contains(u, "TEXT") {
		return "STRING"
	}
	return u // ggf. leer
}

func normalizeFormat(f string) string {
	return strings.TrimSpace(f)
}

// Suche einfache Aufzählungen im Feld-Material (z. B. "Value list", "Permitted values", etc.)
func extractValues(fields map[string]string) []ValueReferencePair {
	var pairs []ValueReferencePair
	for k, v := range fields {
		if strings.Contains(k, "value list") || strings.Contains(k, "permitted values") || strings.Contains(k, "enumeration") {
			// Split nach Komma/Semikolon/Zeilenumbruch
			tokens := splitList(v)
			for _, t := range tokens {
				if t == "" {
					continue
				}
				// Optional: IRDI in Klammern erkennen, z. B. "On (0112/2///61360_4#ABCD12)"
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
	// Muster: "Text (0112/2///61360_4#AAA123)"
	if i := strings.LastIndex(s, "("); i > 0 && strings.HasSuffix(s, ")") {
		val = strings.TrimSpace(s[:i])
		id = strings.TrimSpace(strings.TrimSuffix(s[i+1:], ")"))
		return
	}
	return strings.TrimSpace(s), ""
}

func saveAsIEC61360XML(ds DataSpecificationIec61360, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = f.WriteString(xml.Header)
	enc := xml.NewEncoder(f)
	enc.Indent("", "  ")
	return enc.Encode(ds)
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Gib die CDD IRDI ein: ")
	userInput, _ := reader.ReadString('\n')
	userInput = strings.TrimSpace(userInput)

	cleaned, err := cleanInput(userInput)
	if err != nil {
		fmt.Printf(" %v\n", err)
		return
	}

	number, ok := extractNumber(userInput)
	if !ok {
		fmt.Println(" Keine gültige Nummer zwischen '///' und '#' gefunden!")
		return
	}

	fullURL := buildURL(number, cleaned)
	fmt.Printf("\n Rufe folgende URL auf:\n%s\n\n", fullURL)

	htmlFilename := fmt.Sprintf("%s_%s.html", number, cleaned)
	iecXMLFilename := fmt.Sprintf("%s_%s.iec61360.xml", number, cleaned)

	node, ok := fetchEnglishSection(fullURL, htmlFilename)
	if ok && node != nil {
		fields := extractFields(node)
		ds := buildIEC61360(fields, userInput) // IRDI als letzter Fallback für preferredName
		if err := saveAsIEC61360XML(ds, iecXMLFilename); err != nil {
			fmt.Printf(" Fehler beim Schreiben der IEC61360-XML: %v\n", err)
		} else {
			fmt.Printf(" IEC61360-XML gespeichert als: %s\n", iecXMLFilename)
		}
	}
}
