package mlxscraper

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sort"

	"strings"

	"time"

	"github.com/PuerkitoBio/goquery"
	uuid "github.com/satori/go.uuid"
	"github.com/underlx/disturbancesmlx/dataobjects"
	"github.com/underlx/disturbancesmlx/scraper"
)

// Scraper is a scraper for the status of Metro de Lisboa
type Scraper struct {
	ticker                 *time.Ticker
	stopChan               chan struct{}
	lines                  map[string]*dataobjects.Line
	previousResponse       []byte
	log                    *log.Logger
	statusCallback         func(status *dataobjects.Status)
	topologyChangeCallback func(scraper.Scraper)
	firstUpdate            bool
	lastUpdate             time.Time

	URL     string
	Network *dataobjects.Network
	Source  *dataobjects.Source
	Period  time.Duration
}

// Begin starts the scraper
func (sc *Scraper) Begin(log *log.Logger,
	statusCallback func(status *dataobjects.Status),
	topologyChangeCallback func(scraper.Scraper)) {
	sc.stopChan = make(chan struct{})
	sc.ticker = time.NewTicker(sc.Period)
	sc.log = log
	sc.statusCallback = statusCallback
	sc.topologyChangeCallback = topologyChangeCallback
	sc.lines = make(map[string]*dataobjects.Line)
	sc.firstUpdate = true

	sc.log.Println("Scraper starting")
	sc.update()
	sc.log.Println("Scraper completed first fetch")
	topologyChangeCallback(sc)
	go sc.scrape()
}

func (sc *Scraper) scrape() {
	sc.update()
	sc.log.Println("Scraper completed second fetch")
	for {
		select {
		case <-sc.ticker.C:
			sc.update()
			sc.log.Println("Scraper fetch complete")
		case <-sc.stopChan:
			return
		}
	}
}

func (sc *Scraper) update() {
	response, err := http.Get(sc.URL)
	if err != nil {
		sc.log.Println(err)
		return
	}
	defer response.Body.Close()
	// making sure they don't troll us
	if response.ContentLength < 1024*1024 {
		var buf bytes.Buffer
		tee := io.TeeReader(response.Body, &buf)
		content, err := ioutil.ReadAll(tee)
		if err != nil {
			sc.log.Println(err)
			return
		}
		if !bytes.Equal(content, sc.previousResponse) {
			sc.log.Printf("New status with length %d\n", len(content))

			doc, err := goquery.NewDocumentFromReader(&buf)
			if err != nil {
				sc.log.Println(err)
				return
			}

			if !sc.firstUpdate {
				// if previousResponse is updated on the first update,
				// status won't be collected on the second update
				sc.previousResponse = content
			}

			newLines := make(map[string]*dataobjects.Line)

			css := doc.Find("style").First().Text()

			doc.Find("table").First().Find("tr").Each(func(i int, s *goquery.Selection) {
				line := s.Find("td").First()

				class, _ := line.Attr("class")
				color := "000000"
				classInCSS := strings.Index(css, class)
				pound := strings.Index(css[classInCSS:], "#")
				pound += classInCSS
				if pound > -1 {
					color = css[pound+1 : pound+7]
				}

				words := strings.Split(line.Find("img").AttrOr("alt", ""), " ")
				if len(words) < 2 {
					sc.log.Println("Could not parse line name")
					return
				}
				lineName := words[1]
				lineID := fmt.Sprintf("%s-%s", sc.Network.ID, strings.ToLower(lineName))
				newLines[lineID] = &dataobjects.Line{
					Name:    lineName,
					ID:      lineID,
					Color:   color,
					Network: sc.Networks()[0],
				}

				if !sc.firstUpdate {
					sc.lastUpdate = time.Now().UTC()
					status := line.Next()

					id, err := uuid.NewV4()
					if err != nil {
						return
					}
					if len(status.Find(".semperturbacao").Nodes) == 0 {
						status := &dataobjects.Status{
							ID:         id.String(),
							Time:       time.Now().UTC(),
							Line:       newLines[lineID],
							IsDowntime: true,
							Status:     status.Find("li").Text(),
							Source:     sc.Source,
						}
						sc.statusCallback(status)
					} else {
						status := &dataobjects.Status{
							ID:         id.String(),
							Time:       time.Now().UTC(),
							Line:       newLines[lineID],
							IsDowntime: false,
							Status:     status.Find("li").Text(),
							Source:     sc.Source,
						}
						sc.statusCallback(status)
					}
				}
			})
			sc.lines = newLines
		}
		sc.firstUpdate = false
	}
}

// End stops the scraper
func (sc *Scraper) End() {
	sc.ticker.Stop()
	close(sc.stopChan)
}

// Networks returns the networks monitored by this scraper
func (sc *Scraper) Networks() []*dataobjects.Network {
	return []*dataobjects.Network{sc.Network}
}

// Lines returns the lines monitored by this scraper
func (sc *Scraper) Lines() []*dataobjects.Line {
	lines := []*dataobjects.Line{}
	for _, v := range sc.lines {
		lines = append(lines, v)
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Name < lines[j].Name
	})
	return lines
}

func (sc *Scraper) LastUpdate() time.Time {
	return sc.lastUpdate
}
