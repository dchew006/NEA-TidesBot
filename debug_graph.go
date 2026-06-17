// test_graph.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const TestOutputImage = "test_tide_chart.png"

func main() {
	// 1. Configure the target test parameters here
	testMonth := "June"
	testDay := "30"

	fmt.Printf("⏳ Simulating pipeline execution for: %s %s...\n", testMonth, testDay)

	// 2. Call the identical render function used by the bot pipeline
	generatedHTMLFile, err := RenderChartForDate(testMonth, testDay)
	if err != nil {
		log.Fatalf("❌ Graphing pipeline rendering failed: %v", err)
	}
	fmt.Printf("✅ HTML generated successfully: %s\n", generatedHTMLFile)

	// 3. Launch headless browser engine using your main.go configurations
	fmt.Println("📸 Launching headless browser to capture snapshot...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u := launcher.New().
		NoSandbox(true).
		Headless(true)

	browser := rod.New().ControlURL(u.MustLaunch()).Context(ctx).MustConnect()
	defer browser.MustClose()

	absPath, err := filepath.Abs(generatedHTMLFile)
	if err != nil {
		log.Fatalf("❌ Failed to resolve absolute HTML path: %v", err)
	}

	page := browser.MustPage("file://" + absPath).MustWaitLoad()

	// Snaps the high resolution DOM element matching the layout box
	el := page.MustElement("#dashboard")
	imgData, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
	if err != nil {
		log.Fatalf("❌ Failed to capture screenshot of dashboard: %v", err)
	}

	err = os.WriteFile(TestOutputImage, imgData, 0644)
	if err != nil {
		log.Fatalf("❌ Failed to write final image file: %v", err)
	}

	fmt.Printf("🚀 Success! Open '%s' to verify your tide curve and colored solunar boxes.\n", TestOutputImage)
}
