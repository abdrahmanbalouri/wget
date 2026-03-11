package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
		saveDir:      expandPath(*prefix),
		saveName:     *output,
		convertLinks: *convert,
	}

	if *background {
		f, err := os.Create(logFileName)
		if err != nil {
			log.Fatal(err)
		}
		_ = f.Close()
		if err := runInBackground(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
		fmt.Println(`Output will be written to "wget-log".`)
		return
	}

	if *inputFile != "" {
		urls = readLines(*inputFile)
	}

	if *rateLimitStr != "" {
		limit, err := parseRateLimit(*rateLimitStr)
		if err != nil {
			log.Fatal(err)
		}
		d.rateLimit = limit
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

	if len(urls) == 0 {
		log.Fatal("missing URL or input file")
	}

	if len(urls) == 1 {
		d.downloadSingle(urls[0])
	} else {
		d.downloadMultiple(urls)
	}
}

func runInBackground(args []string) error {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "-B" || arg == "--B" {
			continue
		}
		filtered = append(filtered, arg)
	}

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		_ = logFile.Close()
		return err
	}

	cmd := exec.Command(exe, filtered...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}

	_ = logFile.Close()
	return cmd.Process.Release()
}
