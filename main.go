package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/pkg/errors"
)

const usage = `cloudflare-2020-systems-engineering-assignment

Usage:
  cloudflare-2020-systems-engineering-assignment --url=<URL> [--profile=<REPEAT>]
  cloudflare-2020-systems-engineering-assignment --help

Options:
  --url=<URL>        URL to visit
  --profile=<REPEAT> Number of times to REPEAT profiling
  --help             Show this screen.
`

var httpStatusCodeRegex = regexp.MustCompile(`HTTP\/1\.[0-9]{1} [0-9]{3} .*\n`)

var dialer = net.Dialer{
	Timeout: time.Minute,
}

func handleError(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		os.Exit(1)
	}
}

func makeRequest(link string, requestNo int, responses [][]byte, errorCodes *[]int64) error {
	linkParsed, err := url.Parse(link)
	if err != nil {
		return errors.Wrap(err, "cannot url.Parse")
	}
	tlsConn, err := tls.DialWithDialer(&dialer, "tcp", fmt.Sprintf("%s:https", linkParsed.Host), nil)
	if err != nil {
		return errors.Wrap(err, "cannot tls.Dial")
	}
	defer tlsConn.Close()
	fmt.Fprintf(tlsConn, "%v %v HTTP/1.0\r\n", http.MethodGet, linkParsed.Path)
	fmt.Fprintf(tlsConn, "Host: %v\r\n", linkParsed.Host)
	fmt.Fprintf(tlsConn, "\r\n")

	data, err := ioutil.ReadAll(tlsConn)
	if err != nil {
		return err
	}
	responses[requestNo] = data
	httpStatus := httpStatusCodeRegex.Find(responses[requestNo])
	httpStatusComps := strings.Split(string(httpStatus), " ")
	if len(httpStatusComps) < 2 {
		return errors.New("invalid HTTP Status")
	}
	httpStatusCode, err := strconv.ParseInt(httpStatusComps[1], 10, 64)
	if err != nil {
		return errors.Wrap(err, "cannot parse HTTP StatusCode")
	}
	if httpStatusCode >= 400 || httpStatusCode < 200 {
		*errorCodes = append(*errorCodes, httpStatusCode)
	}
	return nil
}

func main() {
	args, _ := docopt.ParseDoc(usage)
	url, err := args.String("--url")
	handleError(err)
	if url != "" {
		repeat, err := args.Int("--profile")
		profiled := true
		if err != nil {
			profiled = false
			repeat = 1
		} else if repeat == 0 {
			return
		}
		responses := make([][]byte, repeat)
		durations := make([]int64, repeat)
		errorCodes := make([]int64, 0)
		errCount := new(int64)
		wg := new(sync.WaitGroup)
		wg.Add(repeat)
		for i := 0; i < repeat; i++ {
			go func(iteration int) {
				start := time.Now()
				if err := makeRequest(url, iteration, responses, &errorCodes); err != nil {
					fmt.Println(err)
					atomic.AddInt64(errCount, 1)
				}
				end := time.Now()
				elapsed := end.Sub(start)
				durations[iteration] = elapsed.Milliseconds()
				wg.Done()
			}(i)
		}
		wg.Wait()
		min := durations[0]
		minSize := len(responses[0])
		maxSize := len(responses[0])
		max := durations[0]
		sum := int64(durations[0])
		for i := 1; i < repeat; i++ {
			if durations[i] < min {
				min = durations[i]
			} else if durations[i] > max {
				max = durations[i]
			}
			if len(responses[i]) < minSize {
				minSize = len(responses[i])
			} else if len(responses[i]) > maxSize {
				maxSize = len(responses[i])
			}
			sum += durations[i]
		}
		if profiled {
			fmt.Println("Number of requests:", repeat)
			fmt.Println("Fastest time (ms):", min)
			fmt.Println("Slowest time (ms):", max)
			fmt.Println("Mean time (ms):", sum/int64(repeat))
			median := durations[repeat/2]
			if repeat%2 == 0 {
				median += durations[repeat/2-1]
				median /= 2
			}
			fmt.Println("Median time (ms):", median)
			fmt.Println("Percentage of requests that succeeded:", (float64(repeat)-float64(*errCount))/float64(repeat)*100)
			fmt.Println("Error codes returned that weren't a success", errorCodes)
			fmt.Println("Size in bytes of the smallest response:", minSize)
			fmt.Println("Size in bytes of the biggest response:", maxSize)
			fmt.Printf("\n\n")
		}
		if !profiled {
			fmt.Println(string(responses[0]))
		}
	}
}
