package skill

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// 把 Skill 文件中的占位符（变量）替换成实际值
//---
//name: hello-skill
//description: 打招呼
//arguments:
//- name: name
//description: 名字
//required: true
//---
//
//你好，${name}！
//今天是 $TODAY
//你输入了 $ARGUMENTS

var (
	nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// ParseSKILLMarkdown 解析 SKILL.md, 得到 SkillFrontmatter
func ParseSKILLMarkdown(raw []byte) (SkillFrontmatter, string, error) {
	var fm SkillFrontmatter
	content := string(raw)

	if !strings.HasPrefix(content, "---") {
		return fm, content, nil
	}

	lines := strings.SplitN(content, "---", 3)
	if len(lines) < 3 {
		return fm, content, ErrInvalidFrontmatter
	}

	if err := yaml.Unmarshal([]byte(lines[1]), &fm); err != nil {
		return fm, lines[2], ErrInvalidFrontmatter
	}

	if !fm.UserInvocable {
		fm.UserInvocable = true
	}

	return fm, lines[2], nil
}

func ValidateFrontmatter(fm *SkillFrontmatter, dirName string) error {
	if fm.Name == "" {
		return ErrInvalidFrontmatter
	}
	if len(fm.Name) > 64 {
		return ErrInvalidFrontmatter
	}
	if !nameRegex.MatchString(fm.Name) {
		return ErrInvalidFrontmatter
	}

	if fm.Name != dirName {
		return ErrNameMismatch
	}

	if fm.Description == "" {
		return ErrInvalidFrontmatter
	}
	if len(fm.Description) > 1024 {
		return ErrInvalidFrontmatter
	}

	for _, arg := range fm.Arguments {
		if arg.Name == "" {
			return ErrInvalidFrontmatter
		}
	}

	return nil
}

func ExtractMetadataFallback(content string) (string, string) {
	lines := strings.Split(content, "\n")
	var title, desc string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") && title == "" {
			title = strings.TrimSpace(trimmed[2:])
			continue
		}
		if title != "" && desc == "" && !strings.HasPrefix(trimmed, "#") {
			desc = trimmed
			continue
		}
		if title != "" && desc != "" {
			break
		}
	}
	return title, desc
}
