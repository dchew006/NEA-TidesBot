package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ScrapeTides fetches the latest tide data from NEA and saves it to tide_data.json
func ScrapeTides() error {
	url := "https://www.nea.gov.sg/weather/tide-timings"
	res, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return fmt.Errorf("failed to parse HTML: %v", err)
	}

	currentTime := time.Now()
	// Map month abbreviations/identifiers (e.g. "Aug", "Sep") to their prefix "YYYY-MM-"
	monthPrefixMap := make(map[string]string)

	// Discover all month tabs (e.g. "Aug 2026", "Sep 2026")
	doc.Find(".tab__nav .tab__nav-item, .tab__nav-item").Each(func(_ int, btn *goquery.Selection) {
		btnText := strings.TrimSpace(btn.Text())
		if t, err := time.Parse("Jan 2006", btnText); err == nil {
			monthAbbr := t.Format("Jan")
			prefix := t.Format("2006-01-")
			monthPrefixMap[monthAbbr] = prefix
			if dataBox, exists := btn.Attr("data-box"); exists && dataBox != "" {
				monthPrefixMap[dataBox] = prefix
			}
		}
	})

	tideMap := make(map[string][]TideReading)

	// Iterate over all tables on the page (can be multiple months)
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		firstTh := strings.TrimSpace(table.Find("th").First().Text())
		if firstTh == "" {
			return
		}

		// Determine the yearMonthPrefix for this table
		prefix, ok := monthPrefixMap[firstTh]
		if !ok {
			// Fallback: parse 3-letter month abbreviation and infer year relative to currentTime
			if t, err := time.Parse("Jan", firstTh); err == nil {
				targetMonth := t.Month()
				year := currentTime.Year()
				if currentTime.Month() == time.December && targetMonth == time.January {
					year++
				}
				prefix = fmt.Sprintf("%04d-%02d-", year, int(targetMonth))
			} else {
				return // Not a recognized month table
			}
		}

		var activeDate string
		var rowspanRemaining int

		// Iterate row-by-row through the specific table's body
		table.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			var cells []string
			var hasRowspan bool
			var rowspanValue int

			tr.Find("td").Each(func(_ int, td *goquery.Selection) {
				cells = append(cells, strings.TrimSpace(td.Text()))
				if rowspanStr, exists := td.Attr("rowspan"); exists {
					if val, err := strconv.Atoi(rowspanStr); err == nil {
						hasRowspan = true
						rowspanValue = val
					}
				}
			})

			if len(cells) == 0 {
				return
			}

			var timeRaw, heightRaw, classRaw string
			if hasRowspan && len(cells) >= 4 {
				dayStr := cells[0]
				if len(dayStr) == 1 {
					dayStr = "0" + dayStr
				}
				activeDate = prefix + dayStr
				rowspanRemaining = rowspanValue - 1
				timeRaw = cells[1]
				heightRaw = cells[2]
				classRaw = cells[3]
			} else if rowspanRemaining > 0 && len(cells) >= 3 {
				timeRaw = cells[0]
				heightRaw = cells[1]
				classRaw = cells[2]
				rowspanRemaining--
			} else {
				return
			}

			if len(timeRaw) == 4 {
				formattedTime := timeRaw[:2] + ":" + timeRaw[2:]
				var heightVal float64
				fmt.Sscanf(heightRaw, "%f", &heightVal)
				if classRaw == "H" || classRaw == "L" {
					tideMap[activeDate] = append(tideMap[activeDate], TideReading{
						Time:           formattedTime,
						Height:         heightVal,
						Classification: strings.TrimSpace(classRaw),
					})
				}
			}
		})
	})

	var dates []string
	for dateKey := range tideMap {
		dates = append(dates, dateKey)
	}
	sort.Strings(dates)

	var allTides []DayTide
	for _, dateKey := range dates {
		allTides = append(allTides, DayTide{
			Date:     dateKey,
			Readings: tideMap[dateKey],
		})
	}

	jsonData, err := json.MarshalIndent(allTides, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	if err := os.WriteFile("tide_data.json", jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write tide_data.json: %v", err)
	}

	fmt.Println("Scraped data successfully")
	return nil
}