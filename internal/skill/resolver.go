package skill

import (
	"fmt"
	"strings"
)

func ResolveVariables(content string, args SkillInvocationArgs) string {
	result := content

	// 1. 命名参数替换
	for k, v := range args.Named {
		result = strings.ReplaceAll(result, fmt.Sprintf("${%s}", k), v)
		result = strings.ReplaceAll(result, fmt.Sprintf("$%s", k), v)
	}

	// 2. $ARGUMENTS 替换
	if len(args.Positional) > 0 {
		result = strings.ReplaceAll(result, "$ARGUMENTS", strings.Join(args.Positional, " "))
	}

	// 3. $0, $1, $2... 替换
	for i, arg := range args.Positional {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i), arg)
	}

	// 4. 环境变量替换
	for k, v := range args.Env {
		result = strings.ReplaceAll(result, fmt.Sprintf("${%s}", k), v)
	}

	return result
}
