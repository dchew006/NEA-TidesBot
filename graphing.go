// graphing.go
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
)

//go:embed fonts/SFMono-Regular.ttf
var sfMonoBytes []byte

const (
	dataFile     = "tide_data.json"
	templateFile = "template.html"
	outputHTML   = "tide_viewer.html"
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
	PeakTimes  []PeakTimeBlock `json:"peakTimes"`
}

type FontBook struct {
	TitleFont    font.Face
	SubTitleFont font.Face
	BadgeFont    font.Face
	StatusFont   font.Face
	GridFont     font.Face
	ReadingFont  font.Face
	PeakFont     font.Face
}

func loadFonts() (*FontBook, error) {
	parseFace := func(ttfBytes []byte, sizeInPx float64) (font.Face, error) {
		f, err := truetype.Parse(ttfBytes)
		if err != nil {
			return nil, err
		}
		return truetype.NewFace(f, &truetype.Options{
			Size:    sizeInPx,
			DPI:     72, // 1:1 with CSS pixels, since 2x retina scaling is handled by canvas context
			Hinting: font.HintingFull,
		}), nil
	}

	monoTTF := sfMonoBytes
	if len(monoTTF) == 0 {
		monoTTF = gomono.TTF
	}

	title, err := parseFace(gobold.TTF, 34)
	if err != nil {
		return nil, err
	}
	sub, err := parseFace(gobold.TTF, 11)
	if err != nil {
		return nil, err
	}
	badge, err := parseFace(monoTTF, 13)
	if err != nil {
		return nil, err
	}
	status, err := parseFace(monoTTF, 13)
	if err != nil {
		return nil, err
	}
	grid, err := parseFace(gobold.TTF, 11)
	if err != nil {
		return nil, err
	}
	reading, err := parseFace(monoTTF, 11)
	if err != nil {
		return nil, err
	}
	peak, err := parseFace(monoTTF, 9.5)
	if err != nil {
		return nil, err
	}

	return &FontBook{
		TitleFont:    title,
		SubTitleFont: sub,
		BadgeFont:    badge,
		StatusFont:   status,
		GridFont:     grid,
		ReadingFont:  reading,
		PeakFont:     peak,
	}, nil
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

	cmd := exec.Command("./solunar/solunar", "-c", "singapore", "-d", dateArg, "-s")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	rawOutput := out.String()

	reSunrise := regexp.MustCompile(`Sunrise\s*:\s*(\d{2}:\d{2})`)
	reSunset := regexp.MustCompile(`Sunset\s*:\s*(\d{2}:\d{2})`)
	reMoonrise := regexp.MustCompile(`Moonrise\s*:\s*(\d{2}:\d{2})`)
	reMoonset := regexp.MustCompile(`Moonset\s*:\s*(\d{2}:\d{2})`)
	rePeaks := regexp.MustCompile(`(?i)peak times\s*:\s*(.*)`)

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

// RenderNativeChart renders the tide chart directly to a PNG image file
func RenderNativeChart(payload TemplatePayload, outputPath string) error {
	fonts, err := loadFonts()
	if err != nil {
		return fmt.Errorf("failed to load fonts: %w", err)
	}

	// 2x Retina Resolution
	const scale = 2.0
	const imgW = 896 * scale
	const imgH = 600 * scale

	dc := gg.NewContext(int(imgW), int(imgH))
	dc.Scale(scale, scale)

	// 1. Full-bleed card background — no rounded corners so the PNG has zero whitespace pixels
	dc.SetColor(color.RGBA{R: 0x12, G: 0x13, B: 0x18, A: 0xff})
	dc.DrawRectangle(0, 0, 896, 600)
	dc.Fill()

	// 2. Header
	const padX = 32.0
	dc.SetFontFace(fonts.SubTitleFont)
	dc.SetColor(color.RGBA{R: 0xf8, G: 0xf8, B: 0xff, A: 0xff})
	dc.DrawString("ZEROHEROS TIDE CHARTER", padX, 48)

	dc.SetFontFace(fonts.TitleFont)
	dc.SetColor(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	dc.DrawString("Singapore Tides", padX, 86)

	// "Timeline for" label
	dc.SetFontFace(fonts.StatusFont)
	dc.SetColor(color.RGBA{R: 0xe0, G: 0xe0, B: 0xe8, A: 0xff})
	dc.DrawString("Timeline for", padX, 120)

	tw, _ := dc.MeasureString("Timeline for")
	badgeX := padX + tw + 12
	const badgeY = 104
	const badgeW = 118
	const badgeH = 26

	// Date Badge Pill
	dc.SetColor(color.RGBA{R: 0x27, G: 0x27, B: 0x2a, A: 0xff})
	dc.DrawRoundedRectangle(badgeX, badgeY, badgeW, badgeH, 4)
	dc.Fill()
	dc.SetColor(color.RGBA{R: 0x52, G: 0x52, B: 0x5b, A: 0xff})
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(badgeX, badgeY, badgeW, badgeH, 4)
	dc.Stroke()

	dc.SetFontFace(fonts.BadgeFont)
	dc.SetColor(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	dc.DrawStringAnchored(payload.TargetDate, badgeX+badgeW/2, badgeY+badgeH/2+1, 0.5, 0.5)

	// Solunar Status (Right-aligned)
	dc.SetFontFace(fonts.StatusFont)
	if len(payload.PeakTimes) > 0 {
		statusText := "Solunar Peak Windows Active"
		sw, _ := dc.MeasureString(statusText)
		rightEdge := 896.0 - padX
		dotX := rightEdge - sw - 14

		dc.SetColor(color.RGBA{R: 0xfb, G: 0xbf, B: 0x24, A: 0xff})
		dc.DrawCircle(dotX, 117, 3.5)
		dc.Fill()

		dc.SetColor(color.RGBA{R: 0xf0, G: 0xf0, B: 0xf5, A: 0xff})
		dc.DrawStringAnchored(statusText, rightEdge, 118, 1.0, 0.5)
	} else {
		dc.SetColor(color.RGBA{R: 0xf0, G: 0xf0, B: 0xf5, A: 0xff})
		dc.DrawStringAnchored("🌙 No active solunar peak periods today", 896-padX, 118, 1.0, 0.5)
	}

	// Header Separator Line
	dc.SetColor(color.RGBA{R: 0x1f, G: 0x21, B: 0x2a, A: 0xff})
	dc.SetLineWidth(1)
	dc.DrawLine(padX, 142, 896-padX, 142)
	dc.Stroke()

	// 3. Coordinate System Setup for Chart Area
	const plotLeft = padX + 35
	const plotTop = 175.0
	const plotRight = 896 - padX - 15
	const plotBottom = 550.0
	const plotW = plotRight - plotLeft
	const plotH = plotBottom - plotTop

	type Reading struct {
		Minutes int
		Height  float64
		RawTime string
	}
	parseTimeToMins := func(t string) int {
		p := strings.Split(t, ":")
		if len(p) != 2 {
			return 0
		}
		h, _ := strconv.Atoi(p[0])
		m, _ := strconv.Atoi(p[1])
		return h*60 + m
	}

	var readings []Reading
	for _, p := range payload.ChartData {
		readings = append(readings, Reading{
			Minutes: parseTimeToMins(p.X),
			Height:  p.Y,
			RawTime: p.X,
		})
	}

	type TimelinePoint struct {
		Minutes int
		Height  float64
	}
	var master []TimelinePoint
	for _, r := range readings {
		master = append(master, TimelinePoint{Minutes: r.Minutes, Height: r.Height})
	}

	const GAP = 360
	if payload.PrevTime != "" {
		master = append([]TimelinePoint{{
			Minutes: parseTimeToMins(payload.PrevTime) - 1440,
			Height:  payload.PrevHeight,
		}}, master...)
	} else if len(readings) > 1 {
		deltaY := readings[1].Height - readings[0].Height
		master = append([]TimelinePoint{{
			Minutes: readings[0].Minutes - GAP,
			Height:  readings[0].Height + deltaY,
		}}, master...)
	} else if len(readings) > 0 {
		master = append([]TimelinePoint{{
			Minutes: readings[0].Minutes - GAP,
			Height:  readings[0].Height,
		}}, master...)
	}

	if payload.NextTime != "" {
		master = append(master, TimelinePoint{
			Minutes: parseTimeToMins(payload.NextTime) + 1440,
			Height:  payload.NextHeight,
		})
	} else if len(readings) > 1 {
		lastIdx := len(readings) - 1
		deltaY := readings[lastIdx].Height - readings[lastIdx-1].Height
		master = append(master, TimelinePoint{
			Minutes: readings[lastIdx].Minutes + GAP,
			Height:  readings[lastIdx].Height + deltaY,
		})
	} else if len(readings) > 0 {
		lastIdx := len(readings) - 1
		master = append(master, TimelinePoint{
			Minutes: readings[lastIdx].Minutes + GAP,
			Height:  readings[lastIdx].Height,
		})
	}

	type WavePoint struct {
		Minutes int
		Y       float64
	}
	var wavePoints []WavePoint
	maxHeightObserved := 0.0
	leftIdx := 0

	for t := 0; t <= 1440; t += 4 {
		for leftIdx < len(master)-1 && t > master[leftIdx+1].Minutes {
			leftIdx++
		}
		l := master[leftIdx]
		r := master[leftIdx+1]
		denom := float64(r.Minutes - l.Minutes)
		frac := 0.0
		if denom != 0 {
			frac = float64(t-l.Minutes) / denom
		}
		h := (l.Height+r.Height)/2.0 + (l.Height-r.Height)/2.0*math.Cos(frac*math.Pi)
		wavePoints = append(wavePoints, WavePoint{Minutes: t, Y: h})
		if h > maxHeightObserved {
			maxHeightObserved = h
		}
	}

	dynamicMaxHeight := math.Max(1.0, math.Round(maxHeightObserved+1.0))

	getX := func(mins int) float64 {
		return plotLeft + (float64(mins)/1440.0)*plotW
	}
	getY := func(val float64) float64 {
		return plotTop + (1.0-(val/dynamicMaxHeight))*plotH
	}

	// 4. Draw Solunar Peak Windows
	for _, peak := range payload.PeakTimes {
		startMins := parseTimeToMins(peak.StartTime)
		endMins := parseTimeToMins(peak.EndTime)
		x1 := math.Max(plotLeft, getX(startMins))
		x2 := math.Min(plotRight, getX(endMins))
		w := math.Max(0, x2-x1)

		isMajor := peak.Type == "Major"
		if isMajor {
			dc.SetColor(color.NRGBA{R: 249, G: 115, B: 22, A: 38}) // rgba(249, 115, 22, 0.15)
		} else {
			dc.SetColor(color.NRGBA{R: 34, G: 197, B: 94, A: 38}) // rgba(34, 197, 94, 0.15)
		}
		dc.DrawRectangle(x1, plotTop, w, plotH)
		dc.Fill()

		if isMajor {
			dc.SetColor(color.NRGBA{R: 249, G: 115, B: 22, A: 102}) // rgba(249, 115, 22, 0.4)
		} else {
			dc.SetColor(color.NRGBA{R: 34, G: 197, B: 94, A: 102})
		}
		dc.SetLineWidth(1)
		dc.DrawRectangle(x1, plotTop, w, plotH)
		dc.Stroke()

		// Solunar Label
		dc.SetFontFace(fonts.PeakFont)
		if isMajor {
			dc.SetColor(color.RGBA{R: 0xff, G: 0xed, B: 0xd5, A: 0xff})
		} else {
			dc.SetColor(color.RGBA{R: 0xdc, G: 0xfc, B: 0xe7, A: 0xff})
		}
		dc.DrawString(fmt.Sprintf("%s (%s)", peak.Type, peak.Time), x1+6, plotTop+16)
	}

	// 5. Draw Grid Lines and Labels
	dc.SetColor(color.NRGBA{R: 255, G: 255, B: 255, A: 25}) // rgba(255, 255, 255, 0.1)
	dc.SetLineWidth(1)
	dc.SetFontFace(fonts.GridFont)

	// Horizontal Y-axis grid lines (0.5m step when max <= 5, else 1.0m)
	yStep := 0.5
	if dynamicMaxHeight > 5 {
		yStep = 1.0
	}
	for val := 0.0; val <= dynamicMaxHeight+0.001; val += yStep {
		y := getY(val)
		dc.SetColor(color.NRGBA{R: 255, G: 255, B: 255, A: 25})
		dc.DrawLine(plotLeft, y, plotRight, y)
		dc.Stroke()

		dc.SetColor(color.RGBA{R: 0xf0, G: 0xf0, B: 0xf5, A: 0xff})
		label := fmt.Sprintf("%.0f m", val)
		if math.Mod(val, 1.0) != 0 {
			label = fmt.Sprintf("%.1f m", val)
		}
		dc.DrawStringAnchored(label, plotLeft-8, y, 1.0, 0.5)
	}

	// Vertical X-axis grid lines (every 2 hours)
	for hr := 0; hr <= 22; hr += 2 {
		x := getX(hr * 60)
		dc.SetColor(color.NRGBA{R: 255, G: 255, B: 255, A: 25})
		dc.DrawLine(x, plotTop, x, plotBottom)
		dc.Stroke()

		dc.SetColor(color.RGBA{R: 0xf0, G: 0xf0, B: 0xf5, A: 0xff})
		timeLabel := fmt.Sprintf("%02d:00", hr)
		dc.DrawStringAnchored(timeLabel, x, plotBottom+16, 0.5, 0.5)
	}

	// 6. Draw Ocean Wave Gradient Fill
	if len(wavePoints) > 0 {
		grad := gg.NewLinearGradient(0, plotTop*scale, 0, plotBottom*scale)
		grad.AddColorStop(0.0, color.NRGBA{R: 14, G: 165, B: 233, A: 80}) // 0.3 opacity
		grad.AddColorStop(0.7, color.NRGBA{R: 14, G: 165, B: 233, A: 15}) // 0.06 opacity
		grad.AddColorStop(1.0, color.NRGBA{R: 14, G: 165, B: 233, A: 0})  // 0.0 opacity

		dc.MoveTo(getX(wavePoints[0].Minutes), getY(wavePoints[0].Y))
		for i := 1; i < len(wavePoints); i++ {
			dc.LineTo(getX(wavePoints[i].Minutes), getY(wavePoints[i].Y))
		}
		dc.LineTo(plotRight, plotBottom)
		dc.LineTo(plotLeft, plotBottom)
		dc.ClosePath()
		dc.SetFillStyle(grad)
		dc.Fill()

		// 7. Draw Ocean Wave Curve Line
		dc.SetColor(color.RGBA{R: 0x0e, G: 0xa5, B: 0xe9, A: 0xff})
		dc.SetLineWidth(4)
		dc.MoveTo(getX(wavePoints[0].Minutes), getY(wavePoints[0].Y))
		for i := 1; i < len(wavePoints); i++ {
			dc.LineTo(getX(wavePoints[i].Minutes), getY(wavePoints[i].Y))
		}
		dc.Stroke()
	}

	// 8. Draw Readings Points & Data Labels
	for _, r := range readings {
		px := getX(r.Minutes)
		py := getY(r.Height)

		// Dark Center Fill
		dc.SetColor(color.RGBA{R: 0x12, G: 0x13, B: 0x18, A: 0xff})
		dc.DrawCircle(px, py, 6)
		dc.Fill()

		// Cyan Outline
		dc.SetColor(color.RGBA{R: 0x38, G: 0xbd, B: 0xf8, A: 0xff})
		dc.SetLineWidth(3)
		dc.DrawCircle(px, py, 6)
		dc.Stroke()

		// Reading Height Label
		dc.SetFontFace(fonts.ReadingFont)
		dc.SetColor(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
		dc.DrawStringAnchored(fmt.Sprintf("%.2f m", r.Height), px, py-14, 0.5, 0.5)
	}

	return dc.SavePNG(outputPath)
}

// RenderChartForDate renders the chart directly to PNG
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

	peaks, err := fetchSolunarPeaks(month, day)
	if err == nil {
		payload.PeakTimes = peaks
	} else {
		fmt.Printf("Warning: Solunar engine parsing issue skipped: %v\n", err)
	}

	outImg := OutputImagePath
	if err := RenderNativeChart(payload, outImg); err != nil {
		return "", fmt.Errorf("failed to render native chart: %w", err)
	}

	return outImg, nil
}

func parseDate(userInput string) (string, error) {
	now := time.Now()
	currentYear := now.Year()
	formattedInput := strings.Title(strings.ToLower(userInput))

	parseWithYear := func(layout, input string) (string, error) {
		t, err := time.Parse(layout, fmt.Sprintf("%d %s", currentYear, input))
		if err != nil {
			return "", err
		}
		if now.Month() == time.December && t.Month() == time.January {
			t = t.AddDate(1, 0, 0)
		}
		return t.Format("2006-01-02"), nil
	}

	if res, err := parseWithYear("2006 Jan 2", formattedInput); err == nil {
		return res, nil
	}
	if res, err := parseWithYear("2006 January 2", formattedInput); err == nil {
		return res, nil
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