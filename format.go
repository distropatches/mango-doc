package main

import (
	"bytes"
	"strings"
	"container/vector"
	"unicode"
	"fmt"
	"time"
	"go/ast"
	"go/doc"
)

type BR struct {
	c, v *vector.StringVector
	bold bool
}

func NewBR() *BR {
	return &BR{&vector.StringVector{}, &vector.StringVector{}, true}
}

func (s *BR) witch() {
	if s.c.Len() == 0 {
		return
	}
	s.v.Push("\"" + strings.Join([]string(*s.c), "") + "\"")
	s.c = &vector.StringVector{}
	s.bold = !s.bold
}

func (s *BR) B(str string) {
	if !s.bold {
		s.witch()
	}
	s.c.Push(str)
}

func (s *BR) R(str string) {
	//fmt.Println("R:", str)
	if s.bold {
		s.witch()
	}
	s.c.Push(str)
	//fmt.Println("c=", s.c)
}

func escape(in []byte) []byte {
	var buf bytes.Buffer
	last := 0
	for _, rune := range bytes.Runes(in) {
		switch rune {
		case int('\n'):
			buf.WriteByte(' ')
			last = int(' ')
			continue
		case int('\\'):
			buf.WriteString("\\e")
		case int('-'):
			buf.WriteByte('\\')
		case int('.'):
			if last == 0 || unicode.IsSpace(last) {
				buf.WriteByte('\\')
			}
		}
		buf.WriteRune(rune)
		last = rune
	}
	return buf.Bytes()
}

type F struct {
	*bytes.Buffer
	BR *BR
}

func Formatter() *F {
	return &F{&bytes.Buffer{}, NewBR()}
}

type M struct {
	*F
	name, version, sec string
	descr              []byte
	sections           []*section
	refs               []string
	pkg                *ast.Package
	docs               *doc.PackageDoc
}

func NewManPage(pkg *ast.Package, docs *doc.PackageDoc) *M {
	//extract information that will be present regardless of man section
	ver := grep_version(pkg)

	//break up the package document, extract a short description
	pas := paragraphs(docs.Doc)
	if len(pas) == 0 {
		fatal("No package documentation")
	}
	sents := sentences(pas[0])
	d := sents[0]
	//if the first paragraph is one sentence, only use it in description;
	//otherwise we leave it where it is to repeat.
	if len(sents) == 1 {
		pas = pas[1:]
	}
	m := &M{
		Formatter(),
		"",
		ver,
		"",
		d,
		sections(pas),
		nil,
		pkg,
		docs,
	}
	m.WriteString(".\\\"    Automatically generated by mango(1)")
	return m
}

func (m *F) br() {
	//fmt.Println("COMMITTING BR")
	m.BR.witch()
	if m.BR.v.Len() == 0 {
		//fmt.Println("nothing to commit")
		return
	}
	m.WriteString(".BR " + strings.Join([]string(*m.BR.v), " ") + "\n")
	//fmt.Println("output: .BR " + strings.Join([]string(*m.BR.v), " ") + "\n")
	m.BR.v = &vector.StringVector{}
	m.BR.bold = true
}

func (m *F) nl() {
	if m.Bytes()[m.Len()-1] != '\n' {
		m.WriteByte('\n')
	}
}

func (m *F) PP() {
	m.nl()
	m.WriteString(".PP\n")
}

func (m *F) IP() {
	m.nl()
	m.WriteString(".IP\n")
}

func (m *F) section(name string) {
	m.nl()
	m.WriteString(".SH \"")
	m.WriteString(name)
	m.WriteString("\"\n")
}

//spit out raw text
func (m *F) text(p []byte) {
	for _, s := range sentences(p) {
		m.nl()
		m.Write(words(s))
	}
}

func (m *F) paras(ps [][]byte) {
	for i, p := range ps {
		if i != 0 {
			m.PP()
		}
		if p[0] == ' ' || p[0] == '\t' {
			m.code(indents(p))
		} else {
			m.text(p)
		}
	}
}

func (m *F) iparas(ps [][]byte) {
	for i, p := range ps {
		if i != 0 {
			m.IP()
		}
		kind, lines, indents := pkind(p)
		if kind == -1 {
			m.code(lines, indents)
		} else {
			m.text(p)
		}
	}
}

//BUG(jmf): code formatter could balk on double spaced code or mishandle
//complex indentation

func (m *F) code(lines [][]byte, indents []int) {
	min := minin(indents)
	for i, in := range indents {
		indents[i] = in - min
	}
	last, cnt := 0, 0
	m.WriteString(".RS")
	for i, line := range lines {
		m.nl()
		line = bytes.TrimSpace(line)
		in := indents[i]
		if len(line) == 0 {
			m.WriteString(".sp\n")
			continue
		}
		if last < in {
			cnt++
			m.WriteString(".RS\n")
		}
		if last > in {
			cnt--
			if cnt < 0 {
				fatal("Impossible indentation")
			}
			m.WriteString(".RE\n")
		}
		m.Write(line)
		m.nl()
		if i != len(lines)-1 {
			m.WriteString(".sp 0\n")
		}
		last = in
	}
	//make sure indentation balances, in case someone used python code
	for cnt++; cnt > 0; cnt-- {
		m.WriteString("\n.RE")
	}
}

func (m *M) do_header(kind string) {
	tm := time.LocalTime().Format("2006-01-02")
	version := m.version
	if version == "" {
		version = tm
	}
	m.WriteString(fmt.Sprintf("\n.TH \"%s\" %s \"%s\" \"version %s\" \"%s\"",
		m.name,
		m.sec,
		tm,
		version,
		kind,
	))
}

func (m *M) do_name() {
	m.section("NAME")
	m.WriteString(m.name)
	m.WriteString(" \\- ")
	m.Write(bytes.TrimSpace(m.descr)) //first sentence
}

func (m *M) do_description() {
	if len(m.sections[0].paras) > 0 {
		m.section("DESCRIPTION")
		m.paras(m.sections[0].paras)
		m.sections = m.sections[1:]
	}
}

func (m *M) user_sections(sx ...string) {
	ns := make([]*section, len(m.sections))
	n := 0
	for _, rsc := range sx {
		for _, sc := range m.sections {
			if sc.name == rsc {
				m.section(sc.name)
				m.paras(sc.paras)
			} else {
				ns[n] = sc
				n++
			}
		}
	}
	m.sections = ns[:n]
}

func (m *M) remaining_user_sections() {
	for _, sec := range m.sections {
		m.section(sec.name)
		m.paras(sec.paras)
	}
}

func (m *M) do_bugs() {
	bs := m.docs.Bugs
	if len(bs) > 0 {
		m.section("BUGS")
		m.text(bytes.TrimSpace([]byte(bs[0])))
		for _, b := range bs[1:] {
			m.PP()
			m.text(bytes.TrimSpace([]byte(b)))
		}
	}
}

func (m *M) _seealso1(s string) {
	m.WriteString(".BR ")
	piv := strings.Index(s, "(")
	m.Write(escape([]byte(s[:piv])))
	m.WriteByte(' ')
	m.WriteString(s[piv:])
}

func (m *M) do_see_also() {
	if len(m.refs) > 0 {
		m.section("SEE ALSO")
		last := len(m.refs) - 1
		for _, s := range m.refs[:last] {
			m._seealso1(s)
			m.WriteString(",\n")
		}
		m._seealso1(m.refs[last])
	}
}