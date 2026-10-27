// Package epub converte arquivos EPUB em blocos normalizados.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	stdhtml "html"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/giovani/readingdrip/internal/blocks"
)

const (
	containerPath      = "META-INF/container.xml"
	maxMetadataBytes   = 2 << 20  // 2 MiB
	maxXHTMLBytes      = 8 << 20  // 8 MiB por documento
	maxTotalXHTMLBytes = 64 << 20 // 64 MiB por EPUB
)

// Result é o resultado da extração de um EPUB.
type Result struct {
	Title    string
	Author   string
	Language string
	Blocks   []blocks.Block
}

// Extract lê um EPUB em memória e devolve os blocos na ordem do spine.
// Imagens e capa ainda não são extraídas nesta fase.
func Extract(src []byte, fallbackTitle string) (*Result, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("epub: arquivo vazio")
	}

	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		return nil, fmt.Errorf("epub: zip inválido: %w", err)
	}
	archive := fs.FS(zr)

	containerData, err := readZip(archive, containerPath, maxMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("epub: %w", err)
	}
	opfPath, err := parseContainer(containerData)
	if err != nil {
		return nil, err
	}

	opfData, err := readZip(archive, opfPath, maxMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("epub: %w", err)
	}
	pkg, err := parsePackage(opfData)
	if err != nil {
		return nil, err
	}

	manifest := make(map[string]manifestItem, len(pkg.Manifest.Items))
	for _, item := range pkg.Manifest.Items {
		if item.ID == "" {
			continue
		}
		manifest[item.ID] = item
	}

	refs := selectSpine(pkg.Spine.ItemRefs)
	if len(refs) == 0 {
		return nil, fmt.Errorf("epub: spine vazio")
	}

	out := &Result{
		Title:    firstNonEmpty(pkg.Metadata.Titles),
		Author:   joinNonEmpty(pkg.Metadata.Creators, ", "),
		Language: firstNonEmpty(pkg.Metadata.Languages),
	}

	seen := make(map[string]bool)
	var total int64
	for _, ref := range refs {
		item, ok := manifest[ref.IDRef]
		if !ok {
			continue
		}
		if hasToken(item.Properties, "nav") {
			continue
		}
		if !isXHTML(item.MediaType) {
			continue
		}

		entry, err := resolveHref(opfPath, item.Href)
		if err != nil {
			return nil, err
		}
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true

		data, err := readZip(archive, entry, maxXHTMLBytes)
		if err != nil {
			return nil, fmt.Errorf("epub: %w", err)
		}
		total += int64(len(data))
		if total > maxTotalXHTMLBytes {
			return nil, fmt.Errorf("epub: conteúdo XHTML acima do limite (%d MiB)", maxTotalXHTMLBytes>>20)
		}

		em := &emitter{}
		if err := extractXHTML(data, em); err != nil {
			return nil, fmt.Errorf("epub: %s: %w", entry, err)
		}
		out.Blocks = append(out.Blocks, em.blocks...)
	}

	if len(out.Blocks) == 0 {
		return nil, fmt.Errorf("epub: nenhum bloco extraído")
	}
	if out.Title == "" {
		out.Title = firstHeading(out.Blocks)
	}
	if out.Title == "" {
		out.Title = strings.TrimSpace(fallbackTitle)
	}
	return out, nil
}

func readZip(archive fs.FS, name string, limit int64) ([]byte, error) {
	f, err := archive.Open(name)
	if err != nil {
		return nil, fmt.Errorf("entrada %q ausente: %w", name, err)
	}
	defer f.Close()

	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("ler %q: %w", name, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("entrada %q acima do limite (%d MiB)", name, limit>>20)
	}
	return b, nil
}

type containerXML struct {
	Rootfiles []rootfileXML `xml:"rootfiles>rootfile"`
}

type rootfileXML struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

func parseContainer(data []byte) (string, error) {
	var c containerXML
	if err := xml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("epub: container.xml inválido: %w", err)
	}

	var fallback string
	for _, rf := range c.Rootfiles {
		if strings.TrimSpace(rf.FullPath) == "" {
			continue
		}
		if fallback == "" {
			fallback = rf.FullPath
		}
		if strings.Contains(strings.ToLower(rf.MediaType), "oebps-package") {
			return cleanArchivePath(rf.FullPath)
		}
	}
	if fallback != "" {
		return cleanArchivePath(fallback)
	}
	return "", fmt.Errorf("epub: container.xml sem rootfile")
}

type packageXML struct {
	Metadata metadataXML `xml:"metadata"`
	Manifest manifestXML `xml:"manifest"`
	Spine    spineXML    `xml:"spine"`
}

type metadataXML struct {
	Titles    []string `xml:"title"`
	Creators  []string `xml:"creator"`
	Languages []string `xml:"language"`
}

type manifestXML struct {
	Items []manifestItem `xml:"item"`
}

type manifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type spineXML struct {
	ItemRefs []spineItem `xml:"itemref"`
}

type spineItem struct {
	IDRef  string `xml:"idref,attr"`
	Linear string `xml:"linear,attr"`
}

func parsePackage(data []byte) (*packageXML, error) {
	var pkg packageXML
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("epub: pacote OPF inválido: %w", err)
	}
	if len(pkg.Manifest.Items) == 0 {
		return nil, fmt.Errorf("epub: manifest vazio")
	}
	return &pkg, nil
}

func selectSpine(refs []spineItem) []spineItem {
	hasLinear := false
	for _, r := range refs {
		if !isLinearNo(r.Linear) {
			hasLinear = true
			break
		}
	}

	out := make([]spineItem, 0, len(refs))
	for _, r := range refs {
		if hasLinear && isLinearNo(r.Linear) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func isLinearNo(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "no")
}

func isXHTML(mediaType string) bool {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	return mt == "application/xhtml+xml" || mt == "text/html"
}

func hasToken(list, want string) bool {
	for _, f := range strings.Fields(strings.ToLower(list)) {
		if f == want {
			return true
		}
	}
	return false
}

func resolveHref(opfPath, href string) (string, error) {
	href = strings.TrimSpace(href)
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href = href[:i]
	}
	if i := strings.IndexByte(href, '?'); i >= 0 {
		href = href[:i]
	}
	if href == "" {
		return "", nil
	}
	if strings.Contains(href, "://") || strings.HasPrefix(href, "//") {
		return "", fmt.Errorf("epub: href externo não suportado: %q", href)
	}
	if decoded, err := url.PathUnescape(href); err == nil {
		href = decoded
	} else {
		return "", fmt.Errorf("epub: href inválido: %q", href)
	}
	href = strings.ReplaceAll(href, "\\", "/")
	if strings.HasPrefix(href, "/") {
		return "", fmt.Errorf("epub: href absoluto: %q", href)
	}

	p := path.Clean(path.Join(path.Dir(opfPath), href))
	if p == "." || p == "/" || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("epub: href fora do pacote: %q", href)
	}
	return p, nil
}

func cleanArchivePath(raw string) (string, error) {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	if p == "." || p == "/" || strings.HasPrefix(p, "../") || strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("epub: caminho inválido: %q", raw)
	}
	return p, nil
}

func firstNonEmpty(values []string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func joinNonEmpty(values []string, sep string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}

func firstHeading(bs []blocks.Block) string {
	for _, b := range bs {
		if b.Kind.IsHeading() && strings.TrimSpace(b.Text) != "" {
			return b.Text
		}
	}
	return ""
}

type emitter struct {
	blocks []blocks.Block
}

func extractXHTML(src []byte, em *emitter) error {
	doc, err := xhtml.Parse(bytes.NewReader(src))
	if err != nil {
		return err
	}
	body := findFirst(doc, "body")
	if body == nil {
		body = doc
	}
	em.walkFlow(body)
	return nil
}

func findFirst(n *xhtml.Node, tag string) *xhtml.Node {
	if n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, tag) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func (e *emitter) walkFlow(n *xhtml.Node) {
	var inline []*xhtml.Node

	flush := func() {
		if len(inline) == 0 {
			return
		}
		e.emitInline(inline)
		inline = nil
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.TextNode {
			if len(inline) == 0 && strings.TrimSpace(c.Data) == "" {
				continue
			}
			inline = append(inline, c)
			continue
		}
		if c.Type != xhtml.ElementNode {
			inline = append(inline, c)
			continue
		}

		tag := strings.ToLower(c.Data)
		switch {
		case isSkipTag(tag):
			flush()
		case tag == "aside" && isFootnote(c):
			flush()
			e.emitFootnote(c)
		case isContainerTag(tag):
			flush()
			e.walkFlow(c)
		case isBlockTag(tag):
			flush()
			e.emitBlock(c)
		default:
			inline = append(inline, c)
		}
	}
	flush()
}

func (e *emitter) emitInline(nodes []*xhtml.Node) {
	if len(nodes) == 1 && nodes[0].Type == xhtml.ElementNode && strings.EqualFold(nodes[0].Data, "img") {
		e.emitFigureNode(nodes[0])
		return
	}
	text := textOfNodes(nodes)
	if text == "" {
		return
	}
	html := "<p>" + renderInlineNodes(nodes) + "</p>"
	e.blocks = append(e.blocks, blocks.Block{
		Kind: blocks.KindParagraph,
		Text: text,
		HTML: html,
	})
}

func (e *emitter) emitFigureNode(n *xhtml.Node) {
	alt := strings.TrimSpace(attrVal(n, "alt"))
	if alt == "" {
		return
	}
	e.blocks = append(e.blocks, blocks.Block{
		Kind: blocks.KindFigure,
		Text: alt,
		HTML: renderFigure(n),
	})
}

func (e *emitter) emitBlock(n *xhtml.Node) {
	tag := strings.ToLower(n.Data)
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		text := textOfNodes([]*xhtml.Node{n})
		if text == "" {
			return
		}
		level := int(tag[1] - '0')
		e.blocks = append(e.blocks, blocks.Block{
			Kind:  headingKind(level),
			Level: level,
			Text:  text,
			HTML:  "<" + tag + ">" + renderInlineNodes([]*xhtml.Node{n}) + "</" + tag + ">",
		})

	case "p", "dt", "dd":
		if tag == "p" {
			if img := onlyImage(n); img != nil {
				e.emitFigureNode(img)
				return
			}
		}
		text := textOfNodes([]*xhtml.Node{n})
		if text == "" {
			return
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindParagraph,
			Text: text,
			HTML: "<p>" + renderInlineNodes([]*xhtml.Node{n}) + "</p>",
		})

	case "li":
		text := textOfNodes([]*xhtml.Node{n})
		if text == "" {
			return
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindListItem,
			Text: text,
			HTML: renderListItem(n),
		})

	case "pre":
		code := rawText(n)
		if strings.TrimSpace(code) == "" {
			return
		}
		lang := codeLanguage(n)
		class := ""
		if lang != "" {
			class = ` class="language-` + stdhtml.EscapeString(lang) + `"`
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindCode,
			Text: code,
			HTML: "<pre><code" + class + ">" + stdhtml.EscapeString(code) + "</code></pre>",
		})

	case "table":
		text := textOfNodes([]*xhtml.Node{n})
		if text == "" {
			return
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindTable,
			Text: text,
			HTML: renderTable(n),
		})

	case "blockquote":
		text := textOfNodes([]*xhtml.Node{n})
		if text == "" {
			return
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindQuote,
			Text: text,
			HTML: "<blockquote>" + renderFlowHTML(n) + "</blockquote>",
		})

	case "figure":
		alt, caption := figureText(n)
		text := alt
		if text == "" {
			text = caption
		}
		if text == "" {
			return
		}
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindFigure,
			Text: text,
			HTML: renderFigure(n),
		})

	case "hr":
		e.blocks = append(e.blocks, blocks.Block{
			Kind: blocks.KindHR,
			HTML: "<hr>",
		})
	}
}

func (e *emitter) emitFootnote(n *xhtml.Node) {
	text := textOfNodes([]*xhtml.Node{n})
	if text == "" {
		return
	}
	e.blocks = append(e.blocks, blocks.Block{
		Kind: blocks.KindFootnote,
		Text: text,
		HTML: "<aside>" + renderFlowHTML(n) + "</aside>",
	})
}

func headingKind(level int) blocks.Kind {
	switch level {
	case 2:
		return blocks.KindH2
	case 3:
		return blocks.KindH3
	case 4:
		return blocks.KindH4
	case 5:
		return blocks.KindH5
	case 6:
		return blocks.KindH6
	default:
		return blocks.KindH1
	}
}

func isSkipTag(tag string) bool {
	switch tag {
	case "script", "style", "template", "noscript", "head", "title", "meta", "link", "iframe", "object", "embed", "svg", "math":
		return true
	}
	return false
}

func isContainerTag(tag string) bool {
	switch tag {
	case "html", "body", "div", "section", "article", "main", "header", "footer", "aside", "ul", "ol", "dl", "details":
		return true
	}
	return false
}

func isBlockTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "dt", "dd", "li", "pre", "table", "blockquote", "figure", "hr":
		return true
	}
	return false
}

func isFootnote(n *xhtml.Node) bool {
	typ := strings.ToLower(attrVal(n, "epub:type"))
	if strings.Contains(typ, "footnote") || strings.Contains(typ, "endnote") {
		return true
	}
	return strings.EqualFold(attrVal(n, "role"), "doc-footnote")
}

func attrVal(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func textOfNodes(nodes []*xhtml.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		appendText(n, &b)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func appendText(n *xhtml.Node, b *strings.Builder) {
	switch n.Type {
	case xhtml.TextNode:
		b.WriteString(n.Data)
	case xhtml.ElementNode:
		tag := strings.ToLower(n.Data)
		if isSkipTag(tag) {
			return
		}
		if tag == "br" {
			b.WriteByte(' ')
			return
		}
		if tag == "img" {
			b.WriteString(attrVal(n, "alt"))
			b.WriteByte(' ')
			return
		}
		boundary := isTextBoundary(tag)
		if boundary {
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			appendText(c, b)
		}
		if boundary {
			b.WriteByte(' ')
		}
	}
}

func isTextBoundary(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "dt", "dd", "li",
		"pre", "table", "caption", "thead", "tbody", "tfoot", "tr", "td", "th",
		"blockquote", "figure", "figcaption", "hr", "div", "section", "article",
		"header", "footer", "aside", "ul", "ol", "dl", "br":
		return true
	}
	return false
}

func renderInlineNodes(nodes []*xhtml.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		renderInline(n, &b)
	}
	return b.String()
}

func renderInline(n *xhtml.Node, b *strings.Builder) {
	switch n.Type {
	case xhtml.TextNode:
		b.WriteString(stdhtml.EscapeString(n.Data))
	case xhtml.ElementNode:
		tag := strings.ToLower(n.Data)
		if isSkipTag(tag) {
			return
		}
		switch tag {
		case "br":
			b.WriteString("<br>")
			return
		case "img":
			alt := attrVal(n, "alt")
			if alt != "" {
				b.WriteString(`<img alt="` + stdhtml.EscapeString(alt) + `">`)
			}
			return
		case "a":
			href := safeHref(attrVal(n, "href"))
			if href == "" {
				break
			}
			b.WriteString(`<a href="` + stdhtml.EscapeString(href) + `" rel="noopener noreferrer">`)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				renderInline(c, b)
			}
			b.WriteString("</a>")
			return
		case "strong", "b":
			renderWrapped(n, "strong", b)
			return
		case "em", "i":
			renderWrapped(n, "em", b)
			return
		case "code", "kbd", "samp":
			renderWrapped(n, "code", b)
			return
		case "sub", "sup", "mark", "small", "del", "ins", "s", "u", "q":
			renderWrapped(n, tag, b)
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderInline(c, b)
	}
}

func renderWrapped(n *xhtml.Node, tag string, b *strings.Builder) {
	b.WriteString("<" + tag + ">")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderInline(c, b)
	}
	b.WriteString("</" + tag + ">")
}

func renderFlowHTML(n *xhtml.Node) string {
	var b strings.Builder
	var inline []*xhtml.Node

	flush := func() {
		if len(inline) == 0 {
			return
		}
		if textOfNodes(inline) != "" {
			b.WriteString("<p>")
			b.WriteString(renderInlineNodes(inline))
			b.WriteString("</p>")
		}
		inline = nil
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode {
			tag := strings.ToLower(c.Data)
			if isSkipTag(tag) {
				flush()
				continue
			}
			if isFlowBlockTag(tag) {
				flush()
				b.WriteString(renderBlockHTML(c))
				continue
			}
		}
		inline = append(inline, c)
	}
	flush()
	return b.String()
}

func isFlowBlockTag(tag string) bool {
	if isBlockTag(tag) {
		return true
	}
	return tag == "ul" || tag == "ol"
}

func renderBlockHTML(n *xhtml.Node) string {
	tag := strings.ToLower(n.Data)
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "<" + tag + ">" + renderInlineNodes([]*xhtml.Node{n}) + "</" + tag + ">"
	case "p", "dt", "dd":
		return "<p>" + renderInlineNodes([]*xhtml.Node{n}) + "</p>"
	case "li":
		return renderListItem(n)
	case "pre":
		code := rawText(n)
		lang := codeLanguage(n)
		class := ""
		if lang != "" {
			class = ` class="language-` + stdhtml.EscapeString(lang) + `"`
		}
		return "<pre><code" + class + ">" + stdhtml.EscapeString(code) + "</code></pre>"
	case "table":
		return renderTable(n)
	case "blockquote":
		return "<blockquote>" + renderFlowHTML(n) + "</blockquote>"
	case "figure":
		return renderFigure(n)
	case "hr":
		return "<hr>"
	case "ul", "ol":
		return renderList(n)
	}
	return renderInlineNodes([]*xhtml.Node{n})
}

func renderListItem(n *xhtml.Node) string {
	var b strings.Builder
	b.WriteString("<li>")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode {
			tag := strings.ToLower(c.Data)
			if isSkipTag(tag) {
				continue
			}
			if isFlowBlockTag(tag) {
				b.WriteString(renderBlockHTML(c))
				continue
			}
		}
		renderInline(c, &b)
	}
	b.WriteString("</li>")
	return b.String()
}

func renderList(n *xhtml.Node) string {
	tag := strings.ToLower(n.Data)
	var b strings.Builder
	b.WriteString("<" + tag + ">")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && strings.EqualFold(c.Data, "li") {
			b.WriteString(renderListItem(c))
		}
	}
	b.WriteString("</" + tag + ">")
	return b.String()
}

func renderTable(n *xhtml.Node) string {
	var b strings.Builder
	b.WriteString("<table>")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode {
			continue
		}
		switch strings.ToLower(c.Data) {
		case "caption":
			b.WriteString("<caption>" + renderInlineNodes([]*xhtml.Node{c}) + "</caption>")
		case "thead", "tbody", "tfoot":
			tag := strings.ToLower(c.Data)
			b.WriteString("<" + tag + ">")
			renderTableRows(c, &b)
			b.WriteString("</" + tag + ">")
		case "tr":
			renderTableRow(c, &b)
		}
	}
	b.WriteString("</table>")
	return b.String()
}

func renderTableRows(n *xhtml.Node, b *strings.Builder) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && strings.EqualFold(c.Data, "tr") {
			renderTableRow(c, b)
		}
	}
}

func renderTableRow(tr *xhtml.Node, b *strings.Builder) {
	b.WriteString("<tr>")
	for cell := tr.FirstChild; cell != nil; cell = cell.NextSibling {
		if cell.Type != xhtml.ElementNode {
			continue
		}
		cellTag := strings.ToLower(cell.Data)
		if cellTag != "td" && cellTag != "th" {
			continue
		}
		b.WriteString("<" + cellTag)
		if span := attrVal(cell, "colspan"); safeNumber(span) {
			b.WriteString(` colspan="` + span + `"`)
		}
		if span := attrVal(cell, "rowspan"); safeNumber(span) {
			b.WriteString(` rowspan="` + span + `"`)
		}
		b.WriteString(">" + renderInlineNodes([]*xhtml.Node{cell}) + "</" + cellTag + ">")
	}
	b.WriteString("</tr>")
}

func renderFigure(n *xhtml.Node) string {
	alt, caption := figureText(n)
	var b strings.Builder
	b.WriteString("<figure>")
	if alt != "" {
		b.WriteString(`<img alt="` + stdhtml.EscapeString(alt) + `">`)
	}
	if caption != "" {
		b.WriteString("<figcaption>" + stdhtml.EscapeString(caption) + "</figcaption>")
	}
	b.WriteString("</figure>")
	return b.String()
}

func onlyImage(n *xhtml.Node) *xhtml.Node {
	var img *xhtml.Node
	var walk func(*xhtml.Node) bool
	walk = func(cur *xhtml.Node) bool {
		if cur.Type == xhtml.TextNode {
			return strings.TrimSpace(cur.Data) == ""
		}
		if cur.Type != xhtml.ElementNode {
			return true
		}
		tag := strings.ToLower(cur.Data)
		if tag == "script" || tag == "style" {
			return true
		}
		if tag == "img" {
			if img != nil {
				return false
			}
			img = cur
			return true
		}
		if tag == "br" {
			return true
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	if !walk(n) {
		return nil
	}
	return img
}

func figureText(n *xhtml.Node) (alt, caption string) {
	var walk func(*xhtml.Node)
	walk = func(cur *xhtml.Node) {
		if cur.Type == xhtml.ElementNode {
			switch strings.ToLower(cur.Data) {
			case "img":
				if alt == "" {
					alt = strings.TrimSpace(attrVal(cur, "alt"))
				}
			case "figcaption":
				if caption == "" {
					caption = textOfNodes([]*xhtml.Node{cur})
				}
			}
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return alt, caption
}

func rawText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(cur *xhtml.Node) {
		if cur.Type == xhtml.TextNode {
			b.WriteString(cur.Data)
			return
		}
		if cur.Type != xhtml.ElementNode {
			return
		}
		tag := strings.ToLower(cur.Data)
		if isSkipTag(tag) {
			return
		}
		if tag == "br" {
			b.WriteString("\n")
			return
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Trim(b.String(), "\n")
}

func codeLanguage(n *xhtml.Node) string {
	var lang string
	var walk func(*xhtml.Node)
	walk = func(cur *xhtml.Node) {
		if lang != "" || cur.Type != xhtml.ElementNode {
			return
		}
		for _, class := range strings.Fields(attrVal(cur, "class")) {
			switch {
			case strings.HasPrefix(class, "language-"):
				lang = strings.TrimPrefix(class, "language-")
			case strings.HasPrefix(class, "lang-"):
				lang = strings.TrimPrefix(class, "lang-")
			}
			if lang != "" {
				return
			}
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sanitizeToken(lang)
}

func sanitizeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '+' || r == '#' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func safeNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func safeHref(raw string) string {
	href := strings.TrimSpace(raw)
	if href == "" {
		return ""
	}
	for _, r := range href {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "", "http", "https", "mailto", "tel":
		return href
	}
	return ""
}
