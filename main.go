package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
)

type content map[string]string

var address string
var sourceFile string
var useStdin bool

func init() {
	rand.Seed(time.Now().Unix())
}

const (
	defaultAddress    = ":2017"
	defaultUseStdin   = false
	defaultSourceFile = ""
)

const humans = `/* TEAM */
Developer: Nick Gerakines
Site: http://ngerakines.me/
Twitter: @ngerakines
Location: Dayton, OH

/* SITE */
Last update: 2017/09/28 (Happy birthday Vanessa!)
Standards: HTML5, CSS3
Software: Visual Studio Code, Golang

/* SOURCE */
Source: https://github.com/ngerakines/wholesome
License: MIT`

const tpl = `
<!doctype html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Wholesome</title>
    <link rel="author" href="/humans.txt" />
    <style>
        body {
            font-family: 'Lucida Console', 'Courier New', monospace;
            font-size: 26pt;
            line-height: 1.2em;
        }
        #content {
            width: 760px;
            text-align: left;
            margin: 1em auto;
        }
        .permalink {
            font-size: .5em;
            color: #ddd;
            line-height: 1em;
        }
        .permalink a {
            text-decoration: none;
            color: inherit;
        }
        .permalink a:hover {
            text-decoration: underline;
        }
	</style>
	<meta property="og:url" content="https://salty-beyond-53241.herokuapp.com/{{ .Hash }}"/>
	<meta property="og:description" content="{{ .Line }}"/>
	<meta property="og:site_name" content="Wholesome Random Stuff"/>
</head>
<body>
    <div id="content">
        <p>{{ .Line }}</p>
        <p class="permalink">
            [<a href="/{{ .Hash }}">permalink</a>]
        </p>
    </div>
</body>
<!-- Like what you see? http://github.com/ngerakines/wholesome -->
</html>`

func main() {
	logger := logrus.New()
	logger.Formatter = &logrus.JSONFormatter{}
	logger.Level = logrus.DebugLevel
	logger.Out = os.Stdout

	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.StringVar(&address, "address", defaultAddress, "The address to bind the server to.")
	fs.BoolVar(&useStdin, "use-stdin", defaultUseStdin, "Read content from STDIN.")
	fs.StringVar(&sourceFile, "source", defaultSourceFile, "Read content from a file.")
	fs.Parse(os.Args[1:])

	var data content
	var err error

	if useStdin {
		data, err = loadStdin()
	}
	if len(sourceFile) > 0 {
		data, err = loadFile(sourceFile)
	}
	if err != nil {
		logger.
			WithFields(logrus.Fields{
				"address":   address,
				"use-stdin": useStdin,
				"source":    sourceFile,
			}).
			WithError(err).
			Fatal("Error loading data.")
	}
	if len(data) == 0 {
		logger.
			WithFields(logrus.Fields{
				"address":   address,
				"use-stdin": useStdin,
				"source":    sourceFile,
			}).
			Fatal("Data is empty")
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	logger.WithField("data", data).Debug("Loaded data.")

	mux := http.NewServeMux()

	mux.HandleFunc("/humans.txt", func(rw http.ResponseWriter, r *http.Request) {
		logger.
			WithFields(logrus.Fields{
				"method": r.Method,
				"uri":    r.URL.Path,
			}).
			Info()
		rw.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(rw, humans)
	})

	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		entry := logger.WithFields(logrus.Fields{
			"method": r.Method,
			"uri":    path,
		})
		if r.Method != "GET" {
			failRequest(entry, rw, errors.New("unsupported method"), 405)
			return
		}
		contentType := parseContentType(path)

		var hash string
		var line string
		hash = parseHash(path)
		if val, ok := data[hash]; ok {
			line = val
		} else {
			hash = keys[rand.Intn(len(keys))]
			line = data[hash]
		}

		switch contentType {
		case "application/json":
			rw.Header().Set("Content-Type", contentType)
			err = renderJSON(rw, hash, line)
		case "text/plain":
			rw.Header().Set("Content-Type", contentType)
			fmt.Fprintln(rw, line)
		default:
			rw.Header().Set("Content-Type", "text/html")
			err = renderHTML(rw, hash, line)
		}

		if err != nil {
			failRequest(entry, rw, err, 500)
			return
		}

		entry.Info()
	})

	h := &http.Server{
		Addr:    formatAddress(address),
		Handler: mux,
	}

	if err := h.ListenAndServe(); err != nil {
		logger.WithError(err).Warning("HTTP Service stopped.")
	}
}

func parseHash(path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".html")
	path = strings.TrimSuffix(path, ".json")
	path = strings.TrimSuffix(path, ".txt")
	return path
}

func parseContentType(path string) string {
	if strings.HasSuffix(path, ".txt") {
		return "text/plain"
	}
	if strings.HasSuffix(path, ".json") {
		return "application/json"
	}
	return "text/html"
}

func failRequest(entry *logrus.Entry, rw http.ResponseWriter, err error, statusCode int) {
	entry.WithError(err).Error("Failed to handle request.")
	http.Error(rw, err.Error(), statusCode)
}

func renderHTML(w io.Writer, hash, line string) error {
	t, err := template.New("webpage").Parse(tpl)
	if err != nil {
		return err
	}
	return t.Execute(w, struct {
		Hash string
		Line string
	}{
		Hash: hash,
		Line: line,
	})
}

func renderJSON(w io.Writer, hash, line string) error {
	c := struct {
		Line string `json:"line"`
		Hash string `json:"hash"`
	}{Line: line, Hash: hash}
	b, err := json.Marshal(&c)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func loadFile(path string) (content, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	return scanContent(scanner)
}

func loadStdin() (content, error) {
	scanner := bufio.NewScanner(os.Stdin)
	return scanContent(scanner)
}

func scanContent(scanner *bufio.Scanner) (content, error) {
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return parseLines(lines)
}

func parseLines(lines []string) (content, error) {
	content := make(map[string]string)
	for _, line := range lines {
		rawHash := md5.Sum([]byte(line))
		hash := hex.EncodeToString(rawHash[:])
		content[hash] = line
	}
	return content, nil
}

func formatAddress(input string) string {
	if strings.HasPrefix(input, ":") {
		return input
	}
	return ":" + input
}
