package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	
	"github.com/PuerkitoBio/goquery"
)

type TideReading struct {
	Time           string  `json:"time"`
	Height         float64 `json:"height"`
	Classification string  `json:"classification"`
}

type DayTide struct {
	Date     string        `json:"date"`
	Readings []TideReading `json:"readings"`
}

func main() {
	url := "https://www.nea.gov.sg/weather/tide-timings"

	res, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatalf("Failed to parse HTML: %v", err)
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
		// The first <th> in the table contains the month abbreviation (e.g., "Jun", "Jul")
		firstTh := strings.TrimSpace(table.Find("th").First().Text())
		if firstTh != currentMonthAbbr {
			return // Skip tables for other months
		}

		processed = true

		// Variables to track state across multiple table rows
		var activeDate string
		var rowspanRemaining int

		// 3. Iterate row-by-row through the specific table's body
		table.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			var cells []string
			var hasRowspan bool
			var rowspanValue int

			// Extract all cell strings in this row
			tr.Find("td").Each(func(_ int, td *goquery.Selection) {
				cells = append(cells, strings.TrimSpace(td.Text()))
				
				// Check if this specific cell defines a rowspan (the day cell)
				if rowspanStr, exists := td.Attr("rowspan"); exists {
					if val, err := strconv.Atoi(rowspanStr); err == nil {
						hasRowspan = true
						rowspanValue = val
					}
				}
			})

			// Skip structural header rows inside tbody 
			// (Header rows use <th> tags, so tr.Find("td") returns 0 cells)
			if len(cells) == 0 {
				return
			}

			var timeRaw, heightRaw, classRaw string

			if hasRowspan && len(cells) >= 4 {
				// Scenario A: It's the start of a day block (e.g., Row 1 of June 1st)
				dayStr := cells[0]
				if len(dayStr) == 1 {
					dayStr = "0" + dayStr
				}
				activeDate = yearMonthPrefix + dayStr
				rowspanRemaining = rowspanValue - 1 // We are consuming the first row now

				timeRaw = cells[1]
				heightRaw = cells[2]
				classRaw = cells[3]
			} else if rowspanRemaining > 0 && len(cells) >= 3 {
				// Scenario B: It's a continuation row spanned by a previous day
				timeRaw = cells[0]
				heightRaw = cells[1]
				classRaw = cells[2]
				rowspanRemaining--
			} else {
				// Out of sync or empty cell row, skip it safely
				return
			}

			// Sanitize and format the parsed data
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

	// Flatten map to ordered array for JSON file output
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

	jsonData, _ := json.MarshalIndent(monthlyTides, "", "  ")
	_ = os.WriteFile("tide_data.json", jsonData, 0644)
	fmt.Println("Scraped data successfully using explicit rowspan tracking!")
}