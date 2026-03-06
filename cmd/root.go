package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/app"
	"github.com/12345nikhilkumars/crictui/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	tickRate int
	matchID  string
)

const (
	defaultTickRateMs = 10000
	minTickRateMs     = 250
	maxTickRateMs     = 300000
)

var rootCmd = &cobra.Command{
	Use:   "crictui",
	Short: "Live cricket scores in your terminal",
	Long:  "Minimal TUI for viewing cricket scoreboards right in your terminal",
	RunE:  runCrictui,
}

func SetVersion(v string) {
	rootCmd.Version = v
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().IntVarP(&tickRate, "tick-rate", "t", defaultTickRateMs, "Sets match details refresh rate in milliseconds")
	rootCmd.Flags().StringVarP(&matchID, "match-id", "m", "0", "ID of the match to follow live")
}

func runCrictui(cmd *cobra.Command, args []string) error {
	if err := validateTickRate(tickRate); err != nil {
		return err
	}

	if matchID != "0" && !isValidMatchID(matchID) {
		return fmt.Errorf("invalid match ID format")
	}

	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	fmt.Print("\nFetching live matches")
	done := make(chan struct{})
	spinnerStopped := make(chan struct{})
	go func() {
		defer close(spinnerStopped)
		frames := []rune(`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`)
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Printf("\rFetching live matches %c", frames[i%len(frames)])
				i++
			}
		}
	}()

	var cricketApp *app.App
	var err error
	if matchID == "0" {
		cricketApp, err = app.New()
	} else {
		id, _ := strconv.ParseUint(matchID, 10, 32)
		cricketApp, err = app.NewWithMatchID(uint32(id))
	}

	close(done)
	<-spinnerStopped
	fmt.Print("\r                                    \r")

	if err != nil {
		return fmt.Errorf("failed to load: %v", err)
	}
	defer cricketApp.Close()

	model := ui.NewModel(cricketApp, tickRate)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running program: %v", err)
	}
	return nil
}

func isValidMatchID(id string) bool {
	_, err := strconv.ParseUint(id, 10, 32)
	return err == nil
}

func validateTickRate(rate int) error {
	switch {
	case rate <= 0:
		return fmt.Errorf("invalid --tick-rate: must be greater than 0ms (got %dms)", rate)
	case rate < minTickRateMs:
		return fmt.Errorf("invalid --tick-rate: %dms is too low; minimum is %dms", rate, minTickRateMs)
	case rate > maxTickRateMs:
		return fmt.Errorf("invalid --tick-rate: %dms is too high; maximum is %dms", rate, maxTickRateMs)
	default:
		return nil
	}
}
