#!/usr/bin/env python3
"""
Hook 工具库
提供常用的 Hook 开发工具和最佳实践
"""

import json
import sys
import logging
from typing import Dict, Any, Callable, Optional
from functools import wraps

# 配置日志（输出到 stderr，避免污染 stdout）
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr
)
logger = logging.getLogger(__name__)


class HookResponseBuilder:
    """响应构建器 - 帮助构建标准的 Hook 响应"""

    @staticmethod
    def allow(message: str = "允许执行",
              extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """构建 allow 响应"""
        response = {"decision": "allow", "message": message}
        if extensions:
            response["extensions"] = extensions
        return response

    @staticmethod
    def deny(message: str,
             extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """构建 deny 响应"""
        response = {"decision": "deny", "message": message}
        if extensions:
            response["extensions"] = extensions
        return response

    @staticmethod
    def modify(data: Dict[str, Any],
               message: str = "数据已修改",
               extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """构建 modify 响应"""
        response = {
            "decision": "modify",
            "message": message,
            "modified_data": data
        }
        if extensions:
            response["extensions"] = extensions
        return response


class HookValidator:
    """数据验证器 - 提供常用的验证函数"""

    @staticmethod
    def required(data: Dict[str, Any], fields: list) -> tuple[bool, str]:
        """检查必需字段"""
        for field in fields:
            if field not in data or data[field] is None:
                return False, f"缺少必需字段: {field}"
        return True, ""

    @staticmethod
    def range_check(value: float, min_val: float, max_val: float) -> tuple[bool, str]:
        """范围检查"""
        if value < min_val or value > max_val:
            return False, f"值 {value} 不在范围 [{min_val}, {max_val}] 内"
        return True, ""

    @staticmethod
    def type_check(value: Any, expected_type: type) -> tuple[bool, str]:
        """类型检查"""
        if not isinstance(value, expected_type):
            return False, f"期望类型 {expected_type.__name__}, 实际类型 {type(value).__name__}"
        return True, ""


def read_request() -> Dict[str, Any]:
    """从 stdin 读取并解析请求"""
    input_data = sys.stdin.read()
    return json.loads(input_data)


def write_response(response: Dict[str, Any]):
    """输出响应到 stdout"""
    print(json.dumps(response, ensure_ascii=False))


def hook_handler(func: Callable) -> Callable:
    """装饰器: 统一处理异常和响应格式"""
    @wraps(func)
    def wrapper(*args, **kwargs):
        try:
            result = func(*args, **kwargs)
            # 确保有 decision 字段
            if "decision" not in result:
                result["decision"] = "allow"
            return result
        except Exception as e:
            logger.exception("Hook execution failed")
            return HookResponseBuilder.deny(f"处理异常: {str(e)}")
    return wrapper


def hook_main(handler: Callable[[Dict[str, Any]], Dict[str, Any]]):
    """Hook 主函数模板"""
    try:
        request = read_request()
        response = handler(request)
        write_response(response)
        sys.exit(0)
    except Exception as e:
        logger.exception("Main hook failed")
        error_response = HookResponseBuilder.deny(f"Main error: {str(e)}")
        write_response(error_response)
        sys.exit(1)
