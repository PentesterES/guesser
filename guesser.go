// Copyright 2017 Jose Selvi <jselvi{at}pentester.es>
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultCmd     = "sh curl.sh"
	defaultRight   = " "
	defaultWrong   = "^"
	defaultCharset = "0123456789abcdef"
	defaultInit    = ""
	defaultThreads = 10
	defaultDelay   = 0
	defaultDebug   = false
)

// Global variable for debugging
var debug = defaultDebug

// Print only if debug activated
func log(message string) {
	if !debug {
		return
	}
	fmt.Println(message)
}

// Dirty trick to run Cmd with unknown amount of params
func run(cmd string, param string) (int, error) {
	log("Executing: " + cmd + " with " + param)

	// Split Cmd
	v := strings.Split(cmd, " ")
	guess := exec.Command(v[0], v[1:]...)

	stdin, _ := guess.StdinPipe()
	io.WriteString(stdin, param+"\n")
	out, err := guess.Output()
	if err != nil {
		return -1, err
	}

	log("Output: " + string(out))
	score, err := strconv.Atoi(strings.Split(string(out), "\n")[0])
	if err != nil {
		return -1, err
	}

	return score, nil
}

// Gets score if "repeat" tries get the same result
func score(cmd string, param string, repeat int) (int, error) {
	res, _ := run(cmd, param)
	log("Score: " + strconv.Itoa(res))
	for i := 0; i < repeat-1; i++ {
		newres, _ := run(cmd, param)
		log("Score: " + strconv.Itoa(newres))
		if res != newres {
			m := "Site seems to be unestable"
			log(m)
			return -1, errors.New(m)
		}
	}
	return res, nil
}

// Gets longest key (more close to get a result)
func sample(m map[string]string) (string, error) {
	var l int
	var key string
	for k := range m {
		if len(k) > l {
			key = k
			l = len(k)
		}
	}
	if l > 0 {
		return key, nil
	}
	return "", errors.New("Empty Set")
}

// Is "s" substring of any result from "m"?
func isAlreadyResult(m map[string]bool, s string) bool {
	for k := range m {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Main func
func main() {
	// Params parsing
	cmd := flag.String("cmd", defaultCmd, "command to run, parameter sent via stdin")
	right := flag.String("right", defaultRight, "term that makes cmd to give a right response")
	wrong := flag.String("wrong", defaultWrong, "term that makes cmd to give a wrong response")
	charset := flag.String("charset", defaultCharset, "charset we use for guessing")
	init := flag.String("init", defaultInit, "Initial search string")
	threads := flag.Int("threads", defaultThreads, "amount of threads to use")
	delay := flag.Int("delay", defaultDelay, "delay between connections")
	debugFlag := flag.Bool("debug", defaultDebug, "print verbose output (debugging)")
	flag.Parse()

	// If debug is activated, we disable the regular output
	debug = *debugFlag
	var quiet = false
	if debug {
		quiet = true
	}

	// Call to the main func
	guessIt(cmd, right, wrong, charset, init, threads, delay, quiet)
}

// Gets arguments from map instead of command line (for testing purposes)
func guessItMap(param map[string]string) map[string]bool {
	var cmd = defaultCmd
	var right = defaultRight
	var wrong = defaultWrong
	var charset = defaultCharset
	var init = defaultInit
	var threads = defaultThreads
	var delay = defaultDelay
	var debugFlag = defaultDebug
	var err error

	for name, value := range param {
		switch name {
		case "cmd":
			cmd = value
		case "right":
			right = value
		case "wrong":
			wrong = value
		case "charset":
			charset = value
		case "init":
			init = value
		case "threads":
			threads, err = strconv.Atoi(value)
			if err != nil {
				threads = defaultThreads
			}
		case "delay":
			delay, err = strconv.Atoi(value)
			if err != nil {
				delay = defaultDelay
			}
		case "debug":
			debugFlag, err = strconv.ParseBool(value)
			if err != nil {
				debug = defaultDebug
			} else {
				debug = debugFlag
			}
		}
	}

	return guessIt(&cmd, &right, &wrong, &charset, &init, &threads, &delay, true)
}

// Real core
func guessIt(cmd, right, wrong, charset, init *string, threads, delay *int, quiet bool) map[string]bool {
	// Check stability
	log("Checking stability: Right Guess")
	scoreRight, err1 := score(*cmd, *right, 5)
	log("Checking stability: Wrong Guess")
	_, err2 := score(*cmd, *wrong, 5)
	if (err1 != nil) || (err2 != nil) {
		if !quiet {
			m := "Unestable"
			log(m)
			fmt.Println(m)
		}
	}

	// Prepare a Set for substrings and a Set for results
	var pending = make(map[string]string)
	var tmp = make(map[string]bool)
	var res = make(map[string]bool)
	var mtx sync.Mutex
	pending[*init] = "->"

	// While no pending strings to test, go for it
	for len(pending) > 0 {
		// Get a key
		key, _ := sample(pending)
		dir := pending[key]
		delete(pending, key)
		log("Next Guess: " + dir)

		// If key is substring from a previous result, continue
		if len(key) > len(*init)+1 && isAlreadyResult(res, key) {
			log(dir + " was substring of a previous result. Next.")
			continue
		}

		// Prepare Wait Group
		var wg sync.WaitGroup
		wg.Add(len(*charset))

		// Goroutines guessing
		for _, r := range *charset {

			// Wait until we have available threads
			for runtime.NumGoroutine() >= (*threads)+1 {
				time.Sleep(100 * time.Millisecond)
			}

			c := string(r)
			go func(pending map[string]string, cmd string, key string, dir string, c string, right int, res map[string]bool) {
				// Call done when gorouting ends
				defer wg.Done()

				// Get term to test
				var term string
				if dir == "->" {
					term = key + c
				} else {
					term = c + key
				}

				// Calculate score
				score, _ := run(cmd, term)
				log("Guessing " + term + " with score " + strconv.Itoa(score))

				// Save results for next iteration
				if score == right {
					log(term + " was a RIGHT guess")
					mtx.Lock()
					pending[term] = dir
					mtx.Unlock()
				} else {
					log(term + " was a wrong guess")
					mtx.Lock()
					tmp[term] = true
					mtx.Unlock()
				}
			}(pending, *cmd, key, dir, c, scoreRight, res)
		}

		// Wait for goroutines to finish
		wg.Wait()

		// If all chars were errors, we reached the start/end of a string
		if len(tmp) == len(*charset) {
			if dir == "->" {
				log("Guessing in <- direction")
				pending[key] = "<-"
			} else {
				log("Finish guessing")
				res[key] = true
				if !quiet {
					fmt.Printf("\r%s\n", key)
				}
			}
		} else {
			if !quiet {
				fmt.Printf("\r%s", key)
			}
		}
		// Clean temporal map
		tmp = make(map[string]bool)
	}

	// Clean the last try
	if !quiet {
		fmt.Printf("\r                                                    \r")
	}

	return res
}
