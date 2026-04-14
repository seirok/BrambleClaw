package agent

import (
	"brambleclaw/logger"
	"os"
	"testing"
)

func TestContextBuilder_BuildStaticCtx(t *testing.T) {
	contextBuilder, err := NewContextBuilder("../workspace")
	if err != nil {
		logger.L().Error().Msg(err.Error())
		return
	}

	staticCtx, err := contextBuilder.BuildStaticCtx()
	if err != nil {
		logger.L().Error().Msg(err.Error())
		return
	}

	err = os.WriteFile("./report.txt", []byte(staticCtx), 0644)
	if err != nil {
		logger.L().Error().Msg(err.Error())
		return
	}

}
