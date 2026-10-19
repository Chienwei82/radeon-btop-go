// Package sessionlog records a sampling session to CSV or JSON Lines, with
// optional zstd compression selected by the filename suffix.
//
// The design constraint is inherited from the reference implementation and is
// the reason there is no single tidy JSON array: recording is an append-only
// stream, so a session that is interrupted still leaves a readable file. CSV
// opens in a spreadsheet without being asked anything; JSON Lines is what jq
// and any line-oriented reader handle.
//
// Compression is inferred from the name: "session.csv.zst" is a CSV that
// happens to be compressed, so the base name decides the format and the
// suffix decides the transport, and neither has to be declared twice.
//
// Every failure is reported as a value, never raised: a full disk or an
// unwritable directory produces one warning and then silence, because a
// recording that takes the monitor down with it has made things worse. The
// file is flushed per record, so a crash costs at most one row.
package sessionlog

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/klauspost/compress/zstd"
)

// compressionSuffixes are the name suffixes that mean "compress this". ".zst"
// is the conventional one and ".zstd" is accepted because people type both.
var compressionSuffixes = []string{".zst", ".zstd"}

// Format is the on-disk record format.
type Format string

const (
	// FormatCSV is one header row followed by one row per device per sample.
	FormatCSV Format = "csv"
	// FormatJSONL is one JSON object per line, so a session streams, greps
	// and truncates without a parser.
	FormatJSONL Format = "jsonl"
)

// Problem is why a log could not be opened, or why it stopped being written.
type Problem string

const (
	// ProblemUnknownSuffix means the name was neither .csv nor .json*; there
	// is no defensible default for a file the user named themselves.
	ProblemUnknownSuffix Problem = "unknown suffix"
	// ProblemOpenFailed means the file or its directory could not be opened.
	ProblemOpenFailed Problem = "could not be opened"
	// ProblemUnwritable means an open log failed mid-session.
	ProblemUnwritable Problem = "not writable"
)

// DefaultZstdLevel is the compression level used when the configuration does
// not name one; it matches zstd's own default.
const DefaultZstdLevel = 3

// maxZstdLevel is the highest level zstd accepts. A level above it is a
// configuration error, not something to clamp: the user asked for a
// compression the library cannot produce.
const maxZstdLevel = 22

// SplitTarget decides format and compression from a log filename.
//
// It returns the format, whether the target is compressed and the base name
// the format was chosen from (the name with any compression suffix removed).
// The error names the reason no format can be chosen.
func SplitTarget(path string) (Format, bool, string, error) {
	name := filepath.Base(path)
	lower := strings.ToLower(name)
	compressed := false
	stem := name
	for _, suffix := range compressionSuffixes {
		if strings.HasSuffix(lower, suffix) {
			compressed = true
			stem = name[:len(name)-len(suffix)]
			break
		}
	}
	switch strings.ToLower(filepath.Ext(stem)) {
	case ".csv":
		return FormatCSV, compressed, stem, nil
	case ".json", ".jsonl", ".ndjson":
		return FormatJSONL, compressed, stem, nil
	default:
		return FormatJSONL, compressed, stem,
			fmt.Errorf("cannot tell the log format from %q; use .csv or .json", name)
	}
}

// SessionLog is an open recording target. Construct it with OpenLog, which
// resolves the format from the filename and reports a problem as a value.
type SessionLog struct {
	Path       string
	Format     Format
	Compressed bool
	// Problem is empty while the log is usable.
	Problem Problem
	// HeaderWritten says whether the CSV header still has to be written. It
	// is seeded from the file's existing size, so reopening a recording
	// appends rows instead of writing a second header in among them.
	HeaderWritten bool
	// Detail carries the underlying error text for a footer warning.
	Detail string

	records int
	file    *os.File
	encoder *zstd.Encoder
	writer  io.Writer
}

// Records is how many records have been written; one record is one device in
// one snapshot.
func (l *SessionLog) Records() int { return l.records }

// IsOpen reports whether the target can still be written.
func (l *SessionLog) IsOpen() bool { return l != nil && l.Problem == "" && l.writer != nil }

// OpenLog opens a recording target. The suffix selects the format and,
// optionally, zstd; a level of 0 means the library default.
//
// A target that cannot be opened comes back as a SessionLog carrying a
// Problem rather than an error, because "recording disabled, here is why" is
// a footer warning, not a reason to refuse to start the monitor.
func OpenLog(path string, zstdLevel int) *SessionLog {
	format, compressed, _, err := SplitTarget(path)
	if err != nil {
		return &SessionLog{
			Path: path, Format: format, Compressed: compressed,
			Problem: ProblemUnknownSuffix, Detail: err.Error(),
		}
	}
	if zstdLevel < 0 || zstdLevel > maxZstdLevel {
		return &SessionLog{
			Path: path, Format: format, Compressed: compressed,
			Problem: ProblemOpenFailed,
			Detail:  fmt.Sprintf("zstd level %d is outside 0-%d", zstdLevel, maxZstdLevel),
		}
	}
	log := &SessionLog{Path: path, Format: format, Compressed: compressed}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return failedLog(log, mkErr)
	}
	file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if openErr != nil {
		return failedLog(log, openErr)
	}
	log.file = file
	if info, statErr := file.Stat(); statErr == nil {
		log.HeaderWritten = info.Size() > 0
	}
	if !compressed {
		log.writer = file
		return log
	}
	level := zstd.SpeedDefault
	if zstdLevel != 0 {
		level = zstd.EncoderLevelFromZstd(zstdLevel)
	}
	encoder, encErr := zstd.NewWriter(file, zstd.WithEncoderLevel(level))
	if encErr != nil {
		_ = file.Close()
		log.file = nil
		return failedLog(log, encErr)
	}
	log.encoder = encoder
	log.writer = encoder
	return log
}

// failedLog marks a log unusable and keeps the reason for the caller.
func failedLog(log *SessionLog, err error) *SessionLog {
	log.Problem = ProblemOpenFailed
	log.Detail = err.Error()
	log.writer = nil
	return log
}

// Write appends one snapshot: every device in it becomes its own row, so a
// row is always exactly one GPU at one instant.
//
// A snapshot with no devices is not a failure and returns true: there is
// simply nothing to record, and the next sample records normally.
func (l *SessionLog) Write(snapshot model.GpuSnapshot) bool {
	if !l.IsOpen() {
		return false
	}
	for i := range snapshot.Devices {
		line, err := l.line(snapshot, &snapshot.Devices[i])
		if err != nil {
			l.fail(err)
			return false
		}
		if _, err := io.WriteString(l.writer, line); err != nil {
			l.fail(err)
			return false
		}
		l.records++
	}
	if l.Compressed {
		l.Flush()
	}
	return true
}

// line renders one record, including its trailing newline, and writes the CSV
// header first when the file does not have one yet.
func (l *SessionLog) line(snapshot model.GpuSnapshot, metrics *model.AmdgpuMetrics) (string, error) {
	if l.Format == FormatJSONL {
		record, err := jsonRecord(snapshot, metrics)
		if err != nil {
			return "", err
		}
		return record + "\n", nil
	}
	var b strings.Builder
	if !l.HeaderWritten {
		for i, column := range CSVColumns {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(csvCell(column))
		}
		b.WriteByte('\n')
		l.HeaderWritten = true
	}
	for i, cell := range csvRow(snapshot, metrics) {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(csvCell(cell))
	}
	b.WriteByte('\n')
	return b.String(), nil
}

// fail makes the log silent for the rest of the session, keeping the reason.
func (l *SessionLog) fail(err error) {
	l.Problem = ProblemUnwritable
	l.Detail = err.Error()
}

// Flush pushes buffered bytes to the operating system. CSV and JSONL rows are
// written straight to the file, so only a compressed target buffers; there the
// flush keeps "a crash costs at most one row" as close to true as a compressed
// frame permits.
func (l *SessionLog) Flush() {
	if l.encoder != nil {
		_ = l.encoder.Flush()
	}
}

// Close finishes the stream and writes the compression frame's epilogue. A
// log left unclosed is a truncated archive rather than a readable file, which
// is why every exit path calls this.
func (l *SessionLog) Close() {
	if l.writer == nil {
		return
	}
	if l.encoder != nil {
		if err := l.encoder.Close(); err != nil && l.Problem == "" {
			l.fail(err)
		}
		l.encoder = nil
	}
	if l.file != nil {
		if err := l.file.Close(); err != nil && l.Problem == "" {
			l.fail(err)
		}
		l.file = nil
	}
	l.writer = nil
}

// ReadLog reads a recorded file back as lines, decompressing when the name
// carries a compression suffix.
//
// It exists so tests and users checking a recording share the suffix rule that
// SplitTarget already encodes, instead of reimplementing it.
func ReadLog(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var reader io.Reader = file
	if compressedName(path) {
		decoder, decErr := zstd.NewReader(file, zstd.WithDecoderConcurrency(1))
		if decErr != nil {
			return nil, decErr
		}
		defer decoder.Close()
		reader = decoder
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return lines, err
	}
	return lines, nil
}

// compressedName reports whether the filename asks for compression.
func compressedName(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	for _, suffix := range compressionSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
