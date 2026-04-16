package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodeSandboxTool_Execute(t *testing.T) {
	// 1. 模拟沙箱服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证路径
		if r.URL.Path != "/run_code" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// 模拟返回逻辑
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		code := reqBody["code"].(string)

		// 根据输入代码模拟不同的返回
		if strings.Contains(code, "panic") {
			w.Write([]byte(`{
				"status": "Success",
				"run_result": {
					"return_code": 1,
					"stdout": "",
					"stderr": "panic: unexpected error",
					"execution_time": 0.001
				}
			}`))
		} else {
			w.Write([]byte(`{
				"status": "Success",
				"run_result": {
					"return_code": 0,
					"stdout": "Hello Test",
					"stderr": "",
					"execution_time": 0.002
				}
			}`))
		}
	}))
	defer server.Close()

	// 2. 初始化工具（重定向地址到模拟服务器）
	// 注意：在正式代码中 SandboxAddr 是常量，测试时建议将其改为变量或通过构造函数传入
	// 这里为了演示，假设我们在 Execute 中使用了 server.URL
	tool := NewCodeSandboxTool()

	t.Run("成功执行代码", func(t *testing.T) {
		args := `{"language": "python", "code": "print('Hello Test')"}`

		// 临时劫持全局地址或在结构体中支持 URL 自定义
		// 这里假设我们简单修改了 Execute 逻辑以适配测试
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("预期无错误，实际得到: %v", err)
		}

		resStr := result.(string)
		if !strings.Contains(resStr, "Hello Test") {
			t.Errorf("输出未包含预期内容，得到: %s", resStr)
		}
		if !strings.Contains(resStr, "状态: Success") {
			t.Errorf("状态解析错误")
		}
	})

	t.Run("执行报错代码", func(t *testing.T) {
		args := `{"language": "go", "code": "panic('error')"}`
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("工具执行本身不应返回 error，应返回格式化的错误信息")
		}

		resStr := result.(string)
		if !strings.Contains(resStr, "退出码: 1") || !strings.Contains(resStr, "panic") {
			t.Errorf("未能正确捕获标准错误或退出码: %s", resStr)
		}
	})

	t.Run("参数解析失败", func(t *testing.T) {
		invalidArgs := `{"language": "python", "code": }` // 错误的 JSON
		_, err := tool.Execute(context.Background(), invalidArgs)
		if err == nil {
			t.Error("预期 JSON 解析失败会报错，但实际未报错")
		}
	})
}
