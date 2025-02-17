package rfc3164

import (
	"bytes"
	"math"
	"time"

	"github.com/jeromer/syslogparser"
)

type Parser struct {
	buff     []byte
	cursor   int
	l        int
	priority syslogparser.Priority
	version  int
	header   header
	message  rfc3164message
	location *time.Location
	hostname string
}

type ParseResult struct {
	Timestamp time.Time
	Hostname  string
	Tag       string
	Content   string
	Priority  int
	Facility  int
	Severity  int
}

type header struct {
	timestamp time.Time
	hostname  string
}

type rfc3164message struct {
	tag     string
	content string
}

func NewParser(buff []byte) *Parser {
	return &Parser{
		buff:     buff,
		cursor:   0,
		l:        len(buff),
		location: time.UTC,
	}
}

func (p *Parser) Location(location *time.Location) {
	p.location = location
}

func (p *Parser) Hostname(hostname string) {
	p.hostname = hostname
}

func (p *Parser) Parse() error {
	pri, err := p.parsePriority()
	if err != nil {
		return err
	}

	hdr, err := p.parseHeader()
	if err != nil {
		return err
	}

	if p.buff[p.cursor] == ' ' {
		p.cursor++
	}

	msg, err := p.parsemessage()
	if err != syslogparser.ErrEOL {
		return err
	}

	p.priority = pri
	p.version = syslogparser.NO_VERSION
	p.header = hdr
	p.message = msg

	return nil
}

func (p *Parser) Dump() syslogparser.LogParts {
	return syslogparser.LogParts{
		"timestamp": p.header.timestamp,
		"hostname":  p.header.hostname,
		"tag":       p.message.tag,
		"content":   p.message.content,
		"priority":  p.priority.P,
		"facility":  p.priority.F.Value,
		"severity":  p.priority.S.Value,
	}
}

func (p *Parser) DumpParseResult() ParseResult {
	return ParseResult{
		Timestamp: p.header.timestamp,
		Hostname:  p.header.hostname,
		Tag:       p.message.tag,
		Content:   p.message.content,
		Priority:  p.priority.P,
		Facility:  p.priority.F.Value,
		Severity:  p.priority.S.Value,
	}
}

func (p *Parser) parsePriority() (syslogparser.Priority, error) {
	return syslogparser.ParsePriority(p.buff, &p.cursor, p.l)
}

func (p *Parser) parseHeader() (header, error) {
	hdr := header{}
	var err error

	ts, err := p.parseTimestamp()
	if err != nil {
		return hdr, err
	}

	hostname, err := p.parseHostname()
	if err != nil {
		return hdr, err
	}

	hdr.timestamp = ts
	hdr.hostname = hostname

	return hdr, nil
}

func (p *Parser) parsemessage() (rfc3164message, error) {
	msg := rfc3164message{}
	var err error

	tag, err := p.parseTag()
	if err != nil {
		return msg, err
	}

	content, err := p.parseContent()
	if err != syslogparser.ErrEOL {
		return msg, err
	}

	msg.tag = tag
	msg.content = content

	return msg, err
}

// https://tools.ietf.org/html/rfc3164#section-4.1.2
func (p *Parser) parseTimestamp() (time.Time, error) {
	var ts time.Time
	var err error
	var tsFmtLen int
	var sub []byte

	tsFmts := []string{
		"Jan 02 15:04:05",
		"Jan  2 15:04:05",
	}

	found := false
	for _, tsFmt := range tsFmts {
		tsFmtLen = len(tsFmt)

		if p.cursor+tsFmtLen > p.l {
			continue
		}

		sub = p.buff[p.cursor : tsFmtLen+p.cursor]
		ts, err = time.ParseInLocation(tsFmt, string(sub), p.location)
		if err == nil {
			found = true
			break
		}
	}

	if !found {
		p.cursor = tsFmtLen

		// XXX : If the timestamp is invalid we try to push the cursor one byte
		// XXX : further, in case it is a space
		if (p.cursor < p.l) && (p.buff[p.cursor] == ' ') {
			p.cursor++
		}

		return ts, syslogparser.ErrTimestampUnknownFormat
	}

	fixTimestampIfNeeded(&ts)

	p.cursor += tsFmtLen

	if (p.cursor < p.l) && (p.buff[p.cursor] == ' ') {
		p.cursor++
	}

	return ts, nil
}

func (p *Parser) parseHostname() (string, error) {
	if p.hostname != "" {
		return p.hostname, nil
	} else {
		return syslogparser.ParseHostname(p.buff, &p.cursor, p.l)
	}
}

// http://tools.ietf.org/html/rfc3164#section-4.1.3
func (p *Parser) parseTag() (string, error) {

	var b byte
	var tag []byte
	var err error
	var enough bool

	previous := p.cursor
	// "The TAG is a string of ABNF alphanumeric characters that MUST NOT exceed 32 characters."
	limit := int(
		math.Min(
			float64(p.l),
			float64(p.cursor+32),
		),
	)

	for p.cursor < limit {
		b = p.buff[p.cursor]

		if b == ' ' {
			p.cursor++
			break
		}

		if b == '[' || b == ']' || b == ':' || enough {
			enough = true
			p.cursor++
			continue
		}

		tag = append(tag, b)
		p.cursor++
	}

	if len(tag) == 0 {
		p.cursor = previous
	}

	return string(tag), err
}

func (p *Parser) parseContent() (string, error) {
	if p.cursor > p.l {
		return "", syslogparser.ErrEOL
	}

	content := bytes.Trim(p.buff[p.cursor:p.l], " ")
	p.cursor += len(content)

	return string(content), syslogparser.ErrEOL
}

func fixTimestampIfNeeded(ts *time.Time) {
	now := time.Now()
	y := ts.Year()

	if ts.Year() == 0 {
		y = now.Year()
	}

	newTs := time.Date(y, ts.Month(), ts.Day(), ts.Hour(), ts.Minute(),
		ts.Second(), ts.Nanosecond(), ts.Location())

	*ts = newTs
}
