// graphing.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"os/exec"
	// "path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// Added structural peak time details for frontend interpolation
type PeakTimeBlock struct {
	Time      string `json:"time"`
	Type      string `json:"type"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type TemplatePayload struct {
	TargetDate string          `json:"targetDate"`
	ChartData  []ChartPoint    `json:"chartData"`
	PrevTime   string          `json:"prevTime"`
	PrevHeight float64         `json:"prevHeight"`
	NextTime   string          `json:"nextTime"`
	NextHeight float64         `json:"nextHeight"`
	PeakTimes  []PeakTimeBlock `json:"peakTimes"` // Injected Solunar payload array
}

func timeToMinutes(tStr string) int {
	parts := strings.Split(tStr, ":")
	if len(parts) != 2 {
		return -9999
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

func minutesToTimeStr(totalMins int) string {
	totalMins = (totalMins + 1440) % 1440
	h := totalMins / 60
	m := totalMins % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

// Fetch and parse solunar peaks for a given month and day argument
func fetchSolunarPeaks(month, day string) ([]PeakTimeBlock, error) {
	dateArg := fmt.Sprintf("%s %s", month, day)
	
	// Fix the path here to point to the binary file, not the sub-directory folder!
	cmd := exec.Command("./solunar/solunar", "-c", "singapore", "-d", dateArg, "-s")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	rawOutput := out.String()

	reSunrise  := regexp.MustCompile(`(?:Sunrise)\s*:\s*(\d{2}:\d{2})`)
	reSunset   := regexp.MustCompile(`(?:Sunset)\s*:\s*(\d{2}:\d{2})`)
	reMoonrise := regexp.MustCompile(`(?:Moonrise)\s*:\s*(\d{2}:\d{2})`)
	reMoonset  := regexp.MustCompile(`(?:Moonset)\s*:\s*(\d{2}:\d{2})`)

	rePeaks    := regexp.MustCompile(`.*Peak times\s*:\s*(.*)`)

	getMatch := func(re *regexp.Regexp, target string) string {
		matches := re.FindStringSubmatch(target)
		if len(matches) > 1 {
			return matches[1]
		}
		return ""
	}

	var anchors []int
	anchors = append(anchors, timeToMinutes(getMatch(reSunrise, rawOutput)))
	anchors = append(anchors, timeToMinutes(getMatch(reSunset, rawOutput)))
	anchors = append(anchors, timeToMinutes(getMatch(reMoonrise, rawOutput)))
	if ms := getMatch(reMoonset, rawOutput); ms != "" {
		anchors = append(anchors, timeToMinutes(ms))
	}

	peaksRaw := getMatch(rePeaks, rawOutput)
	peakTokens := strings.Fields(peaksRaw)
	var processedPeaks []PeakTimeBlock

	for _, peak := range peakTokens {
		peakMins := timeToMinutes(peak)
		// SAFELY IGNORE INVALID OR EMPTY ARRAYS AND "(none)" STRING TOKENS
		if peakMins < 0 {
			continue
		}

		peakType := "Major"
		for _, anchorMins := range anchors {
			if anchorMins < 0 {
				continue
			}
			if int(math.Abs(float64(peakMins-anchorMins))) <= 35 {
				peakType = "Minor"
				break
			}
		}

		offset := 60
		if peakType == "Minor" {
			offset = 30
		}

		processedPeaks = append(processedPeaks, PeakTimeBlock{
			Time:      peak,
			Type:      peakType,
			StartTime: minutesToTimeStr(peakMins - offset),
			EndTime:   minutesToTimeStr(peakMins + offset),
		})
	}
	return processedPeaks, nil
}

// Updated entry method signature accepting explicit partitioned strings
func RenderChartForDate(month, day string) (string, error) {
	userInput := fmt.Sprintf("%s %s", month, day)
	targetDate, err := parseDate(userInput)
	if err != nil {
		return "", fmt.Errorf("invalid date format: %w", err)
	}

	allTides, err := loadTideData(dataFile)
	if err != nil {
		return "", err
	}

	matchedIdx := findTideIndex(allTides, targetDate)
	if matchedIdx == -1 {
		return "", fmt.Errorf("no tide data found for date: %s", targetDate)
	}

	payload := buildPayload(matchedIdx, allTides, targetDate)

	// Inject the live dynamic calculated Solunar Peaks into the runtime model structure
	peaks, err := fetchSolunarPeaks(month, day)
	if err == nil {
		payload.PeakTimes = peaks
	} else {
		fmt.Printf("Warning: Solunar engine parsing issue skipped: %v\n", err)
	}

	if err := renderTemplate(templateFile, outputFile, payload); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return outputFile, nil
}

func parseDate(userInput string) (string, error) {
	currentYear := time.Now().Year()
	formattedInput := strings.Title(strings.ToLower(userInput))
	dateStr := fmt.Sprintf("%d %s", currentYear, formattedInput)

	if t, err := time.Parse("2006 Jan 2", dateStr); err == nil {
		return t.Format("2006-01-02"), nil
	}
	if t, err := time.Parse("2006 January 2", dateStr); err == nil {
		return t.Format("2006-01-02"), nil
	}
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