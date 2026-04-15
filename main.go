package main

import (
	"brambleclaw/cli"
	"brambleclaw/logger"
)

// TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>
func main() {
	if err := cli.Execute(); err != nil {
		logger.L().Fatal().Err(err).Msg("CLI执行失败")
	}
}
