#!/usr/bin/env python3
"""
示例 2: 订单审计 Hook
使用 hook_utils 工具库
"""

import sys
import logging

# 添加脚本路径以导入 hook_utils
import os
sys.path.insert(0, os.path.dirname(__file__))

from hook_utils import (
    read_request,
    write_response,
    hook_handler,
    HookResponseBuilder,
    HookValidator,
    hook_main
)

logger = logging.getLogger(__name__)


@hook_handler
def audit_order(request: dict) -> dict:
    """处理订单审计"""
    data = request.get("data", {})

    # 1. 验证必需字段
    ok, msg = HookValidator.required(data, ["order_id", "amount", "user_id"])
    if not ok:
        return HookResponseBuilder.deny(msg)

    # 2. 验证金额类型
    amount = data.get("amount", 0)
    ok, msg = HookValidator.type_check(amount, (int, float))
    if not ok:
        return HookResponseBuilder.deny(msg)

    # 3. 检查金额是否超限
    if amount > 10000:
        return HookResponseBuilder.deny(
            f"金额 {amount} 超过单笔限额 10000",
            {"reason": "amount_limit_exceeded"}
        )

    # 4. 大额订单自动打折
    if amount > 5000:
        modified_data = data.copy()
        discount = amount * 0.05  # 5% 折扣
        modified_data["amount"] = round(amount - discount, 2)
        modified_data["discount"] = round(discount, 2)
        modified_data["discount_rate"] = 0.05

        logger.info(f"Applied 5% discount to order {data.get('order_id')}")

        return HookResponseBuilder.modify(
            modified_data,
            f"已自动应用 5% 折扣，优惠 {discount:.2f} 元",
            {
                "discount_amount": discount,
                "original_amount": amount,
                "final_amount": modified_data["amount"]
            }
        )

    # 5. 正常通过
    return HookResponseBuilder.allow(
        "订单审核通过",
        {
            "order_id": data.get("order_id"),
            "user_id": data.get("user_id"),
            "amount": amount
        }
    )


if __name__ == "__main__":
    hook_main(audit_order)
