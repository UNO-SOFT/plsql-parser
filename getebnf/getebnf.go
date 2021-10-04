package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/fetchbot"
	"github.com/PuerkitoBio/goquery"
	"github.com/goccy/go-json"
)

var (
	// Protect access to dup
	dupMu sync.RWMutex
	// Duplicates table
	dup = make(map[string]struct{})

	// Command-line flags
	flagSeed = flag.String("seed", "https://docs.oracle.com/en/database/oracle/oracle-database/21/lnpls/index.html https://docs.oracle.com/en/database/oracle/oracle-database/21/sqlrf/index.html", "seed URLs (space separated)")
	// https://docs.oracle.com/en/database/oracle/oracle-database/21/lnpls/
)

func main() {
	if err := Main(); err != nil {
		log.Fatalf("ERROR: %+v", err)
	}
}

func Main() error {
	flag.Parse()

	type description struct {
		Path, Description string
	}

	// Create the muxer
	mux := fetchbot.NewMux()

	var q *fetchbot.Queue

	// Handle all errors the same
	mux.HandleErrors(fetchbot.HandlerFunc(func(ctx *fetchbot.Context, res *http.Response, err error) {
		log.Printf("[ERR] %s %s - %s\n", ctx.Cmd.Method(), ctx.Cmd.URL(), err)
	}))

	// Parse the provided seed
	seeds := strings.Fields(*flagSeed)
	us := make([]*url.URL, 0, len(seeds))
	hosts := make([]string, cap(us))
	for _, seed := range seeds {
		u, err := url.Parse(seed)
		if err != nil {
			return fmt.Errorf("%q: %w", seed, err)
		}
		us = append(us, u)
		hosts = append(hosts, u.Host)
	}

	enc := json.NewEncoder(os.Stdout)

	for _, u := range us {
		// Handle GET requests for html responses, to parse the body and enqueue all links as HEAD
		// requests.
		mux.Response().Method("GET").Host(u.Host).ContentType("text/html").Handler(fetchbot.HandlerFunc(
			func(ctx *fetchbot.Context, res *http.Response, err error) {
				if err != nil {
					log.Println(ctx.Cmd.URL(), err)
					return
				}
				log.Println("Handling", ctx.Cmd.URL())
				// Process the body to find the links
				doc, err := goquery.NewDocumentFromReader(res.Body)
				if err != nil {
					log.Printf("[ERR] %s %s - %s\n", ctx.Cmd.Method(), ctx.Cmd.URL(), err)
					return
				}
				/*
					<body>
					      <article>
						           <header>
								               <h1>Description of the illustration accessible_by_clause.eps</h1>
											            </header>
														         <div><pre
				*/
				doc.Find("body>article").Each(func(i int, s *goquery.Selection) {
					if strings.HasPrefix(s.Find("header>h1").Text(), "Description ") {
						desc := description{
							Path:        ctx.Cmd.URL().Path,
							Description: s.Find("div>pre").Text(),
						}
						if desc.Description == "" {
							return
						}
						if err := enc.Encode(desc); err != nil {
							log.Println("ERROR:", err)
							_ = q.Cancel()
						}
					}
				})
				// Enqueue all links as HEAD requests
				log.Println("enqueue", ctx.Cmd.URL())
				enqueueLinks(ctx, hosts, doc)
			}))
	}

	// Create the Fetcher, handle the logging first, then dispatch to the Muxer
	f := fetchbot.New(mux)
	f.CrawlDelay = 100 * time.Millisecond
	f.AutoClose = true

	log.Printf("Start")
	// Start processing
	q = f.Start()

	// Enqueue the seed, which is the first entry in the dup map
	for _, u := range us {
		seed := u.String()
		dup[seed] = struct{}{}
		_, err := q.SendStringGet(seed)
		if err != nil {
			log.Printf("[ERR] GET %s - %s\n", seed, err)
		}
	}
	q.Block()
	return q.Close()
}

func enqueueLinks(ctx *fetchbot.Context, matchHosts []string, doc *goquery.Document) {
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		val, _ := s.Attr("href")
		// Resolve address
		u, err := ctx.Cmd.URL().Parse(val)
		if err != nil {
			log.Printf("error: resolve URL %s - %s\n", val, err)
			return
		}
		if !(u.Scheme == "http" || u.Scheme == "https") {
			return
		}
		ok := len(matchHosts) == 0
		for _, host := range matchHosts {
			if ok = host == u.Host; ok {
				break
			}
		}
		if !ok {
			return
		}
		u.Fragment, u.RawFragment = "", ""
		k := u.String()
		dupMu.RLock()
		_, ok = dup[k]
		dupMu.RUnlock()
		if !ok {
			dupMu.Lock()
			if _, ok = dup[k]; !ok {
				dup[k] = struct{}{}
				_, _ = ctx.Q.SendStringGet(u.String())
			}
			dupMu.Unlock()
		}
	})
}
