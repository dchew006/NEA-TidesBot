// scraper.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	yearMonthPrefix := currentTime.Format("2006-01-")
	currentMonthAbbr := currentTime.Format("Jan") // e.g., "Jun", "Jul"

	tideMap := make(map[string][]TideReading)
	processed := false

	// 1. Iterate over all tables on the page
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		if processed {
			return // Stop if we've already found and processed the current month's table
		}

		// 2. Check if this table is for the current month
		firstTh := strings.TrimSpace(table.Find("th").First().Text())
		if firstTh != currentMonthAbbr {
			return // Skip tables for other months
		}
		processed = true

		var activeDate string
		var rowspanRemaining int

		// 3. Iterate row-by-row through the specific table's body
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
				activeDate = yearMonthPrefix + dayStr
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

	var monthlyTides []DayTide
	for d := 1; d <= 31; d++ {
		dayStr := strconv.Itoa(d)
		if len(dayStr) == 1 {
			dayStr = "0" + dayStr
		}
		dateKey := yearMonthPrefix + dayStr
		if readings, exists := tideMap[dateKey]; exists {
			monthlyTides = append(monthlyTides, DayTide{
				Date:     dateKey,
				Readings: readings,
			})
		}
	}

	jsonData, err := json.MarshalIndent(monthlyTides, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	if err := os.WriteFile("tide_data.json", jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write tide_data.json: %v", err)
	}

	fmt.Println("Scraped data successfully")
	return nil
}