package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/gojektech/heimdall"
	"github.com/gojektech/heimdall/v6/hystrix"
	"github.com/gojektech/heimdall/v6/plugins"
)

// hystrixClient type is used for sending http requests
type hystrixClient struct {
	Client *hystrix.Client
	Once   sync.Once
}

// HystrixClient is an instance of hystrixClient
var HystrixClient = &hystrixClient{}

func (h *hystrixClient) InitHystrixClient() {
	h.Once.Do(func() {
		initalTimeout := 2 * time.Millisecond         // Inital timeout
		maxTimeout := 9 * time.Millisecond            // Max time out
		exponentFactor := 2.0                         // Multiplier
		maximumJitterInterval := 2 * time.Millisecond // Max jitter interval. It must be more than 1*time.Millisecond
		backoff := heimdall.NewExponentialBackoff(initalTimeout, maxTimeout, exponentFactor, maximumJitterInterval)
		// Create a new retry mechanism with the backoff
		retrier := heimdall.NewRetrier(backoff)
		timeout := 1000 * time.Millisecond
		// Create a new client, sets the retry mechanism, and the number of times you would like to retry
		h.Client = hystrix.NewClient(
			hystrix.WithHTTPTimeout(timeout),
			hystrix.WithCommandName("updating-vertical-on-status-change"),
			hystrix.WithHystrixTimeout(50*time.Millisecond),
			hystrix.WithMaxConcurrentRequests(100),
			hystrix.WithErrorPercentThreshold(10),
			hystrix.WithSleepWindow(100),
			hystrix.WithRequestVolumeThreshold(20),
			hystrix.WithRetryCount(1),
			hystrix.WithRetrier(retrier),
			hystrix.WithFallbackFunc(func(err error) error {
				return err
			}),
		)
		requestLogger := plugins.NewRequestLogger(nil, nil)
		h.Client.AddPlugin(requestLogger)
	})
}

func main() {
	HystrixClient.InitHystrixClient()

	// Create an http.Request instance
	httpRequest, err := http.NewRequest(http.MethodGet, "https://google.com", nil)
	if err != nil {
		fmt.Println(fmt.Errorf(err.Error()))
	}

	resp, err := HystrixClient.Client.Do(httpRequest)
	if err != nil {
		fmt.Println(fmt.Errorf(err.Error()))
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(string(body))
}
