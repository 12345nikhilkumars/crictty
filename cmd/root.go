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
	rootCmd.Flags().IntVarP(&tickRate, "tick-rate", "t", 40000, "Sets match details refresh rate in milliseconds")
	rootCmd.Flags().StringVarP(&matchID, "match-id", "m", "0", "ID of the match to follow live")
}

func runCrictui(cmd *cobra.Command, args []string) error {
	if matchID != "0" && !isValidMatchID(matchID) {
		return fmt.Errorf("invalid match ID format")
	}

	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	fmt.Print("\nFetching live matches")
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				for _, r := range `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` {
					fmt.Printf("\rFetching live matches %c", r)
					time.Sleep(100 * time.Millisecond)
				}
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

	done <- true
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
