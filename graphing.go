// graphing.go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"
	"path/filepath"
)

const (
	dataFile     = "tide_data.json"
	templateFile = "template.html"
	outputFile   = "tide_viewer.html"
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

type ChartPoint struct {
	X string  `json:"x"`
	Y float64 `json:"y"`
}

type TemplatePayload struct {
	TargetDate string        `json:"targetDate"`
	ChartData  []ChartPoint  `json:"chartData"`
	PrevTime   string        `json:"prevTime"`
	PrevHeight float64       `json:"prevHeight"`
	NextTime   string        `json:"nextTime"`
	NextHeight float64       `json:"nextHeight"`
}

// RenderChartForDate acts as the bridging engine for the telegram bot loop
func RenderChartForDate(userInput string) (string, error) {
	targetDate, err := parseDate(userInput)
	if err != nil {
		return "", fmt.Errorf("invalid date format: %w", err)
	}

	allTides, err := loadTideData(dataFile)
	if err != nil {
		return "", err
	}

	absPath, _ := filepath.Abs(templateFile)
    fmt.Printf("Bot is loading template from: %s\n", absPath)

	matchedIdx := findTideIndex(allTides, targetDate)
	if matchedIdx == -1 {
		return "", fmt.Errorf("no tide data found for date: %s", targetDate)
	}

	payload := buildPayload(matchedIdx, allTides, targetDate)

	if err := renderTemplate(templateFile, outputFile, payload); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return outputFile, nil
}

func parseDate(userInput string) (string, error) {
	currentYear := time.Now().Year()
	formattedInput := strings.Title(strings.ToLower(userInput))
	dateStr := fmt.Sprintf("%d %s", currentYear, formattedInput)

	// 1. Try abbreviated month layout first (handles "Jun 10", "Jan 5", etc.)
	if t, err := time.Parse("2006 Jan 2", dateStr); err == nil {
		return t.Format("2006-01-02"), nil
	}

	// 2. Try full month name layout (handles "June 10", "January 5", etc.)
	if t, err := time.Parse("2006 January 2", dateStr); err == nil {
		return t.Format("2006-01-02"), nil
	}

	// If both fail, return a generic error
	return "", fmt.Errorf("could not parse date %q", userInput)
}

func loadTideData(filename string) ([]DayTide, error) {
	fileData, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%s not found: %v", filename, err)
	}

	var allTides []DayTide
	if err := json.Unmarshal(fileData, &allTides); err != nil {
		return nil, fmt.Errorf("error parsing JSON data: %v", err)
	}
	return allTides, nil
}

func findTideIndex(allTides []DayTide, targetDate string) int {
	for i, day := range allTides {
		if day.Date == targetDate {
			return i
		}
	}
	return -1
}

func buildPayload(matchedIdx int, allTides []DayTide, targetDate string) TemplatePayload {
	matchedDay := allTides[matchedIdx]

	points := make([]ChartPoint, 0, len(matchedDay.Readings))
	for _, r := range matchedDay.Readings {
		points = append(points, ChartPoint{X: r.Time, Y: r.Height})
	}

	payload := TemplatePayload{
		TargetDate: targetDate,
		ChartData:  points,
	}

	if matchedIdx > 0 {
		prevReadings := allTides[matchedIdx-1].Readings
		if len(prevReadings) > 0 {
			last := prevReadings[len(prevReadings)-1]
			payload.PrevTime = last.Time
			payload.PrevHeight = last.Height
		}
	}

	if matchedIdx < len(allTides)-1 {
		nextReadings := allTides[matchedIdx+1].Readings
		if len(nextReadings) > 0 {
			first := nextReadings[0]
			payload.NextTime = first.Time
			payload.NextHeight = first.Height
		}
	}

	return payload
}

func renderTemplate(tmplFile, outFile string, payload TemplatePayload) error {
	tmpl, err := template.ParseFiles(tmplFile)
	if err != nil {
		return fmt.Errorf("failed to parse html template: %v", err)
	}

	out, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create viewer file: %v", err)
	}
	defer out.Close()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload to JSON: %v", err)
	}

	templateData := map[string]interface{}{
		"TargetDate": payload.TargetDate,
		"Payload":    template.JS(payloadJSON),
	}

	return tmpl.Execute(out, templateData)
}
