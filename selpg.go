package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	flag "github.com/spf13/pflag"
)

// some args
var (
	// mandatory options
	startPage = flag.IntP("start", "s", 0, "Page number of the file where you want to print start from. (must be positive)")
	endPage   = flag.IntP("end", "e", 0, "Page number of the file where you want to print end to. (must be positive)")

	// optional options
	limitLine     = flag.IntP("limit", "l", 72, "Line number for one page.")
	pagebreakFlag = flag.BoolP("pbflag", "f", false, "Flag to find page break or not.")
	destination   = flag.StringP("destination", "d", "", "Printer destination to print choesn page.")
)

var (
	pageendFlag = byte('\n')
	limitFlag   = 72
)

// system variable
var (
	exitCode = 0
)

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: selpg -s <start-page> -e <end-page> [option...] -- <file_path>")
	flag.PrintDefaults()
}

// initial flag here
func init() {
	flag.CommandLine.SortFlags = false
	flag.Usage = usage
	/*
		flag.CommandLine.MarkDeprecated("start", "This flag has been deprecated")
		flag.CommandLine.MarkDeprecated("end", "This flag has been deprecated")
		flag.CommandLine.MarkDeprecated("limit", "This flag has been deprecated")
		flag.CommandLine.MarkDeprecated("pbflag", "This flag has been deprecated")
		flag.CommandLine.MarkDeprecated("destination", "This flag has been deprecated")
	*/
}

// utils
func processStream(in io.Reader, out io.Writer) error {
	// process input stream
	pageIter, flagIter, writedFlag := 1, 0, false

	// deal page with flag '\f'
	buffer := make([]byte, 16)
	n, err := in.Read(buffer)

	for err == nil {
		accStart, accEnd := 0, n

		for i := 0; i < n; i++ {
			// count iterator
			if pageendFlag == buffer[i] {

				flagIter = (flagIter + 1) % limitFlag
				// next page
				if flagIter == 0 {
					pageIter++
					// find output interval in byte buffer.
					if pageIter == *startPage {
						accStart = i + 1
					} else if pageIter == *endPage+1 {
						accEnd = i + 1
					}
				}
			}
		}

		if pageIter >= *startPage {
			writedFlag = true
			io.WriteString(out, string(buffer[accStart:accEnd]))
		}
		if pageIter > *endPage {
			break
		}
		n, err = in.Read(buffer)
	}
	/*
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			if pageIter >= *startPage && pageIter <= *endPage {
				if pageIter != *startPage && flagIter == 0 {
					io.WriteString(out, "\f")
				}
				io.WriteString(out, scanner.Text())
			} else if pageIter > *endPage {
				break
			}
			flagIter = (flagIter + 1) % limitFlag
			if flagIter == 0 {
				pageIter++
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	*/
	if writedFlag {
		return nil
	} else {
		return errors.New("usage: page number out of file range or input stream is empty.")
	}
}

// printer goroutine
func runPrinter(reader io.Reader, quit chan error) {
	cmd := exec.Command("lp", "-d", *destination)
	cmd.Stdin = reader

	// create command standard output and input output reader
	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		quit <- err
		return
	}

	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		quit <- err
		return
	}

	// start command and wait
	if err := cmd.Start(); err != nil {
		quit <- err
		return
	}
	if _, err := io.Copy(os.Stdout, stdoutReader); err != nil {
		quit <- err
		return
	}
	if _, err := io.Copy(os.Stderr, stderrReader); err != nil {
		quit <- err
		return
	}
	if err := cmd.Wait(); err != nil {
		quit <- err
		return
	}
	quit <- nil
}

func reportErr(err error) {
	exitCode = 2
	fmt.Fprintln(os.Stderr, err)
	usage()
}

// main process
func main() {
	// warp selpgMain() function, so defer won't be execute after os.Exit(exitCode)
	selpgMain()
	os.Exit(exitCode)
}

func selpgMain() {
	// check flag correction
	flag.Parse()
	shortFlag := make(map[string]int)
	flag.Visit(func(f *flag.Flag) {
		shortFlag[f.Shorthand] = 1
	})

	if shortFlag["l"] == 1 && shortFlag["f"] == 1 {
		reportErr(errors.New("usage: arguments -l and -f can not be set at the same time!"))
		return
	}
	if shortFlag["e"] == 0 || shortFlag["s"] == 0 {
		reportErr(errors.New("usage: rguments -s and -e is needed!"))
		return
	} else if *startPage <= 0 || *endPage <= 0 || *startPage > *endPage {
		reportErr(errors.New("usage: rguments -s and -e must be positive, and argument -e must be equal or greater than -s"))
		return
	}

	limitFlag = *limitLine
	if *pagebreakFlag {
		limitFlag = 1
		pageendFlag = byte('\f')
	}

	// switch output writer to (os.Stdout | lp -d)
	var out io.Writer
	quit := make(chan error)
	if *destination == "" {
		out = os.Stdout
	} else {
		// create lp printer to the destination
		reader, writer := io.Pipe()
		out = writer
		go runPrinter(reader, quit)
		defer func() {
			writer.Close()
			if err := <-quit; err != nil {
				exitCode = 2
				log.Fatal(err)
			}
		}()
	}

	// process input from stdin
	if flag.NArg() == 0 {
		// check stdin input mode, do not accept ModeCharDevice mode
		stat, err := os.Stdin.Stat()
		if err != nil {
			exitCode = 2
			log.Fatal(err)
		}
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			reportErr(errors.New("usage: invalid standard input!"))
			return
		}
		// process stdin stream
		if err := processStream(os.Stdin, out); err != nil {
			reportErr(err)
		}
		return
	}

	// process input file from file name
	path := flag.Arg(0)
	f, err := os.Open(path)
	defer f.Close()
	if _, err2 := f.Stat(); err2 != nil || err != nil {
		reportErr(err)
		return
	}
	if err := processStream(f, out); err != nil {
		reportErr(err)
		return
	}
}
