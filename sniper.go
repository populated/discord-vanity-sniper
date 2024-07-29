package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/valyala/fasthttp"
)

type Config struct {
	Token   string `json:"token"`
	Guild   int    `json:"guild"`
	Vanity  string `json:"vanity"`
	Threads int    `json:"threads"`
}

type Sniper struct {
	config   Config
	client   *fasthttp.Client
	atts     int
	snipe    bool
	mu       sync.Mutex
	start    time.Time
}

type ErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func NewSniper() *Sniper {
	configFile, err := os.Open("./data/config.json")
	if err != nil {
		fmt.Println("Error opening config file:", err)
		os.Exit(1)
	}
	defer configFile.Close()

	var config Config
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		fmt.Println("Error decoding config file:", err)
		os.Exit(1)
	}

	return &Sniper{
		config: config,
		client: &fasthttp.Client{},
		snipe:  true,
	}
}

func (s *Sniper) headers() map[string]string {
	return map[string]string{
		"authority":        "discord.com",
		"x-discord-locale": "en-US",
		"x-debug-options":  "bugReporterEnabled",
		"accept-language":  "en-US",
		"authorization":    s.config.Token,
		"user-agent":       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"accept":           "*/*",
		"origin":           "https://discord.com",
		"sec-fetch-site":   "same-origin",
		"sec-fetch-mode":   "cors",
		"sec-fetch-dest":   "empty",
	}
}

func (s *Sniper) snipeVanity(workerID int, results chan<- bool, wg *sync.WaitGroup) {
	defer wg.Done()

	for s.snipe {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)

		req.SetRequestURI(fmt.Sprintf("https://discord.com/api/v9/invites/%s", s.config.Vanity))
		req.Header.SetMethod("GET")

		for key, value := range s.headers() {
			req.Header.Set(key, value)
		}

		var err error
		for retries := 0; retries < 3; retries++ {
			err = s.client.Do(req, resp)
			if err == nil {
				break
			}
			fmt.Println("Error making request:", err)
		}
		if err != nil {
			continue
		}

		s.mu.Lock()
		s.atts++
		if s.atts%200 == 0 {
			elapsed := time.Since(s.start).Seconds()
			rps := float64(s.atts) / elapsed
			fmt.Println(aurora.Yellow(fmt.Sprintf("WARNING [Worker %d]: Failed to snipe vanity after %d attempts. Continuing... RPS: %.2f", workerID, s.atts, rps)))
		}
		s.mu.Unlock()

		body := resp.Body()
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errorResp.Message == "Unknown Invite" && errorResp.Code == 10006 {
				s.snipe = false
				results <- true
				break
			}
		}
	}

	results <- false
}

func (s *Sniper) claim() {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(fmt.Sprintf("https://discord.com/api/v9/guilds/%d/vanity-url", s.config.Guild))
	req.Header.SetMethod("PATCH")
	req.Header.SetContentType("application/json")

	for key, value := range s.headers() {
		req.Header.Set(key, value)
	}

	req.SetBodyString(fmt.Sprintf(`{"code": "%s"}`, s.config.Vanity))

	err := s.client.Do(req, resp)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}

	if resp.StatusCode() == fasthttp.StatusOK {
		elapsed := time.Since(s.start)
		fmt.Println(aurora.Magenta(fmt.Sprintf("SUCCESS: Vanity successfully sniped. - vanity=%s - attempts=%d - time=%s", s.config.Vanity, s.atts, elapsed)))
	} else {
		fmt.Println(aurora.Red(fmt.Sprintf("ERROR: Failed to snipe vanity. - vanity=%s - attempts=%d", s.config.Vanity, s.atts)))
	}
}

func (s *Sniper) run() {
	var wg sync.WaitGroup
	results := make(chan bool)

	s.start = time.Now()

	for i := 0; i < s.config.Threads; i++ {
		wg.Add(1)
		go s.snipeVanity(i, results, &wg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result {
			s.claim()
			break
		}
	}
}

func main() {
	sniper := NewSniper()
	sniper.run()
}
