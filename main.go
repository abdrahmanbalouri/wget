package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const logFileName = "wget-log"

func main() {
	background := flag.Bool("B", false, "background")
	output := flag.String("O", "", "output name")
	prefix := flag.String("P", ".", "output directory")
	rateLimitStr := flag.String("rate-limit", "", "rate limit")
	inputFile := flag.String("i", "", "input file")
	mirror := flag.Bool("mirror", false, "mirror website")
	reject := flag.String("R", "", "reject extensions")
	exclude := flag.String("X", "", "exclude paths")
	convert := flag.Bool("convert-links", false, "convert links")

	flag.Parse()
	urls := flag.Args()

	d := &Downloader{
		outWriter:    os.Stdout,
		errWriter:    os.Stderr,
		saveDir:      *prefix,
		saveName:     *output,
		convertLinks: *convert,
	}

	if *background {
		f, err := os.Create(logFileName)
		if err != nil {
			log.Fatal(err)
		}
		d.outWriter = f
		d.errWriter = f
		fmt.Println(`Output will be written to "wget-log".`)
	}

	if *rateLimitStr != "" {
		d.rateLimit = parseRateLimit(*rateLimitStr)
	}

	d.startTime = time.Now()
	fmt.Fprintf(d.outWriter, "start at %s\n", d.startTime.Format("2006-01-02 15:04:05"))

	if *mirror {
		if len(urls) == 0 {
			log.Fatal("mirror requires URL")
		}
		d.mirrorWebsite(
			urls[0],
			strings.Split(*reject, ","),
			strings.Split(*exclude, ","),
		)
		return
	}

	if *inputFile != "" {
		urls = readLines(*inputFile)
	}

	if len(urls) == 1 {
		d.downloadSingle(urls[0])
	} else {
		d.downloadMultiple(urls)
	}
}
