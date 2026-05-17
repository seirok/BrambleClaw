#!/usr/bin/env python3
"""
示例 1: 简单的 Hello Hook
"""

import sys
import json


def main():
    try:
        # 读取请求
        request = json.loads(sys.stdin.read())
        data = request.get("data", {})

        # 获取 name 参数
        name = data.get("name", "World")

        # 构建响应
        response = {
            "decision": "allow",
            "message": f"👋 Hello, {name}!",
            "extensions": {
                "processed": True,
                "hook_name": "hello_hook"
            }
        }

        # 输出响应
        print(json.dumps(response))
        sys.exit(0)

    except Exception as e:
        error_response = {
            "decision": "deny",
            "message": f"Error: {str(e)}"
        }
        print(json.dumps(error_response))
        sys.exit(1)


if __name__ == "__main__":
    main()
