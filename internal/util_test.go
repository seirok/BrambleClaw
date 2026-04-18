package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsFirstLevelSubDir(t *testing.T) {
	// 1. 准备测试环境：创建一个临时根目录
	tempRoot, err := os.MkdirTemp("", "test_path_logic")
	if err != nil {
		t.Fatalf("无法创建临时目录: %v", err)
	}
	defer os.RemoveAll(tempRoot) // 测试结束后清理

	// 2. 在临时目录下创建实际的目录结构
	// 结构：
	// tempRoot/
	//    ├── child1/
	//    │     └── grandchild/
	//    └── file1.txt (这是一个文件，不是目录)
	child1 := filepath.Join(tempRoot, "child1")
	grandchild := filepath.Join(child1, "grandchild")
	file1 := filepath.Join(tempRoot, "file1.txt")

	os.MkdirAll(grandchild, 0755)
	os.WriteFile(file1, []byte("hello"), 0644)

	// 3. 定义测试用例
	tests := []struct {
		name     string
		parent   string
		sub      string
		expected bool
	}{
		{"正常一级子目录", tempRoot, child1, true},
		{"相同目录", tempRoot, tempRoot, false},
		{"二级子目录", tempRoot, grandchild, false},
		{"父级目录的外部目录", child1, tempRoot, false},
		{"是一个存在的文件而非目录", tempRoot, file1, false},
		{"物理上不存在的目录", tempRoot, filepath.Join(tempRoot, "non_existent"), false},
		{"相对路径形式的一级目录", tempRoot, "./" + filepath.Base(child1), true},
		{"带点号的复杂路径", tempRoot, child1 + "/../child1", true},
	}

	// 4. 执行测试
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 如果测试用例中使用的是相对路径，需要切换工作目录或处理路径
			// 这里为了简单，我们在 tests 中处理好绝对路径关系
			got, err := IsFirstLevelSubDir(tt.parent, tt.sub)
			if err != nil {
				t.Errorf("函数返回了非预期错误: %v", err)
			}
			if got != tt.expected {
				t.Errorf("IsFirstLevelSubDir(%s, %s) = %v; want %v",
					tt.parent, tt.sub, got, tt.expected)
			}
		})
	}
}
