package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/logrusorgru/aurora"
)

type Config struct {
	Token   string `json:"token"`
	Guild   int    `json:"guild"`
	Vanity  string `json:"vanity"`
	Threads int    `json:"threads"`
}

type Sniper struct {
	config   Config
	client   *http.Client
	atts     int
	snipe    bool
	proxies  []string
	mu       sync.Mutex
	start    time.Time
	proxyIdx int
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

	proxies, err := ioutil.ReadFile("./data/proxies.txt")
	if err != nil {
		fmt.Println("Error reading proxies file:", err)
		os.Exit(1)
	}

	return &Sniper{
		config:  config,
		client:  &http.Client{},
		snipe:   true,
		proxies: strings.Split(string(proxies), "\n"),
	}
}

func (s *Sniper) createClient() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.proxies) > 0 {
		proxyURL, err := url.Parse(s.proxies[s.proxyIdx])
		if err != nil {
			fmt.Println("Error parsing proxy URL:", err)
			return
		}
		s.client.Transport = &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			IdleConnTimeout:     30 * time.Second,
		}
		s.proxyIdx = (s.proxyIdx + 1) % len(s.proxies)
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

	// I have no clue why /invites/%s is being proxied, considering it has a global rate limit.
	// So even if it's proxied, it doesn't matter.

	for s.snipe {
		req, err := http.NewRequest("GET", fmt.Sprintf("https://discord.com/api/v9/invites/%s", s.config.Vanity), nil)
		if err != nil {
			fmt.Println("Error creating request:", err)
			continue
		}

		for key, value := range s.headers() {
			req.Header.Set(key, value)
		}

		var resp *http.Response
		for retries := 0; retries < 3; retries++ {
			resp, err = s.client.Do(req)
			if err == nil {
				break
			}
			fmt.Println("Error making request:", err)
			s.createClient()
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
		
		body, err := ioutil.ReadAll(resp.Body)
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if errorResp.Message == "Unknown Invite" && errorResp.Code == 10006 {
				s.snipe = false
				results <- true
				resp.Body.Close()
				break
			}
		}

		resp.Body.Close()
	}

	results <- false
}

func (s *Sniper) claim() {
	reqBody := strings.NewReader(fmt.Sprintf(`{"code": "%s"}`, s.config.Vanity))
	req, err := http.NewRequest("PATCH", fmt.Sprintf("https://discord.com/api/v9/guilds/%d/vanity-url", s.config.Guild), reqBody)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	for key, value := range s.headers() {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
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
	sniper.createClient()
	sniper.run()
}
