package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/afex/hystrix-go/hystrix"
	"github.com/eapache/go-resiliency/retrier"
)

func main() {
	// Build Circuit Breaker
	cb := CircuitBreaker{
		Timeout: 10000,
		Retry:   1,
	}

	// Build Http Request
	request, err := http.NewRequest(http.MethodGet, "https://webhook.site/80f98c80-5cce-4fce-9eaa-c578d65ab886", nil)
	if err != nil {
		log.Println(err.Error())
	}

	// Call Http with circuit breaker
	res, err := cb.CallUsingCircuitBreaker("test", request, nil)
	if err != nil {
		log.Println(err.Error())
	}

	log.Println(string(res))
}

var (
	// DefaultTimeout in Millisecond
	DefaultTimeout = 5000
	// DefaultRetry in Count
	DefaultRetry = 1
	// DefaultBackoff in Millisecond
	DefaultBackoff = 0
)

// CircuitBreaker model
type CircuitBreaker struct {
	Timeout int // Millisecond
	Retry   int // Count
	Backoff int // Millisecond - Wait time each request
}

// CallUsingCircuitBreaker call http with circuit breaker
func (cb CircuitBreaker) CallUsingCircuitBreaker(breakername string, req *http.Request, body []byte) ([]byte, error) {
	timeout := DefaultTimeout
	if cb.Timeout != 0 {
		timeout = cb.Timeout
	}

	retry := DefaultRetry
	if cb.Retry != 0 {
		retry = cb.Retry
	}

	backoff := DefaultBackoff
	if cb.Backoff != 0 {
		backoff = cb.Backoff
	}

	maxTimeout := timeout + (timeout * retry) + (backoff * retry)

	hystrix.ConfigureCommand(breakername, hystrix.CommandConfig{
		Timeout: maxTimeout,
	})

	output := make(chan []byte, 1) // declare the channel where the hystrix goroutine will put success responses.

	errors := hystrix.Go(breakername, // pass the name of the circuit breaker as first parameter.

		// 2nd parameter, the inlined func to run inside the breaker.
		func() error {
			// for hystrix, forward the err from the retrier. it's nil if successful.
			return cb.callWithRetries(req, output, body)
		},

		// 3rd parameter, the fallback func. in this case, we just do a bit of logging and return the error.
		func(err error) error {
			log.Println(fmt.Sprintf("in fallback function for breaker %v, error: %v", breakername, err.Error()))
			circuit, _, _ := hystrix.GetCircuit(breakername)
			log.Println(fmt.Sprintf("circuit state is: %v", circuit.IsOpen()))
			return err
		})
	// response and error handling. if the call was successful, the output channel gets the response.
	// otherwise, the errors channel gives us the error.
	select {
	case out := <-output:
		log.Println(fmt.Sprintf("call in breaker %v successful", breakername))
		return out, nil
	case err := <-errors:
		log.Println(fmt.Sprintf("call in breaker %v error: %v", breakername, err))
		return nil, err
	}
}

func (cb CircuitBreaker) callWithRetries(req *http.Request, output chan []byte, body []byte) error {
	timeout := DefaultTimeout
	if cb.Timeout != 0 {
		timeout = cb.Timeout
	}

	retries := DefaultRetry
	if cb.Retry != 0 {
		retries = cb.Retry
	}

	backoff := DefaultBackoff
	if cb.Backoff != 0 {
		backoff = cb.Backoff
	}

	clientTimeout := time.Duration(timeout * int(time.Millisecond))
	backoffDuration := time.Duration(backoff * int(time.Millisecond))

	// create a retrier with constant backoff, retries number of attempts (n) with a XXXms sleep between retries.
	r := retrier.New(retrier.ConstantBackoff(retries, backoffDuration), retrier.DefaultClassifier{})

	// this counter is just for getting some logging for showcasing, remove in production code.
	attempt := 0

	// retrier works similar to hystrix, we pass the actual work (doing the http request) in a func.
	err := r.Run(func() error {
		// do http request and handle response. if successful, pass resp.body over output channel,
		// otherwise, do a bit of error logging and return the err.
		var client = &http.Client{
			Timeout: clientTimeout,
		}
		if body != nil {
			req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		}

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			responseBody, err := ioutil.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err == nil {
				output <- responseBody
				return nil
			}
		} else if err == nil {
			err = fmt.Errorf("status was %v", resp.StatusCode)
		}

		attempt++
		log.Println(fmt.Sprintf("retrier failed, attempt %v, response: %v, error: %v", attempt, resp, err))
		return err
	})
	return err
}
