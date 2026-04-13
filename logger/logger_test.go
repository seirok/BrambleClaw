package logger

import (
	"testing"
)

func TestLogger(t *testing.T) {
	Init("test.log", "debug")
	if err := doSomething(); err != nil {
		L().Debug().Msg(err.Error())
	}
	SetLevel("info")
	if err := doSomething(); err != nil {
		L().Debug().Msg(err.Error()) // 不应该再有输出
	}

}

type TestError struct {
}

func (err TestError) Error() string {
	return "some error"
}
func doSomething() error {
	return TestError{}
}
