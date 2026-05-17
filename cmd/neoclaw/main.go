package main

import (
	"neoclaw/internal/cli"
	"neoclaw/internal/logger"
)

// TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>
func main() {
	if err := cli.Execute(); err != nil {
		logger.L().Fatal().Err(err).Msg("CLI execution failed")
	}
}
