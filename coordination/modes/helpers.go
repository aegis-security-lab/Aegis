package modes

import (
	"encoding/json"
	"fmt"
	"strings"

	"aegis/coordination"
)

func payload(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("coordination/modes: encode effect: %w", err)
	}
	return encoded, nil
}

func decode[T any](raw json.RawMessage) (T, error) {
	var value T
	if len(raw) == 0 || string(raw) == "null" {
		return value, nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, fmt.Errorf("coordination/modes: decode payload: %w", err)
	}
	return value, nil
}

func effect(kind coordination.EffectType, key string, value any) (coordination.PlannedEffect, error) {
	raw, err := payload(value)
	return coordination.PlannedEffect{Type: kind, IdempotencyKey: key, Payload: raw}, err
}

func one(item coordination.PlannedEffect, err error) ([]coordination.PlannedEffect, error) {
	if err != nil {
		return nil, err
	}
	return []coordination.PlannedEffect{item}, nil
}

func completionMessage(event coordination.Event, completion coordination.Completion) string {
	message := strings.TrimSpace(completion.Message)
	if message == "" {
		message = strings.TrimSpace(completion.Result)
	}
	if message == "" && completion.Error != "" {
		message = completion.Error
	}
	if message == "" {
		message = fmt.Sprintf("Child work %s completed.", event.IssueID)
	}
	if completion.Status == "budget_exceeded" {
		message = fmt.Sprintf("子 Issue %s 的本次 Execution 已超出预算，并已结束为可分析的业务结果。\n\n%s\n\n请父 Issue 明确判断下一步：\n1. 若方向仍值得继续，调用 coordinate_continue 为同一 Issue 创建预算重新计数的新 Execution；\n2. 若当前方向价值不足，调用 coordinate_delegate 创建新的 Issue 探索其他方向；\n3. 若现有证据已足够，接受部分结果并继续父 Issue。\n\n注意：新 Execution 不会重置 Task 的总时钟墙预算。", event.IssueID, message)
	}
	if completion.Status == "failed" {
		message = fmt.Sprintf("子 Issue %s 执行失败，父 Issue 不应继续等待其他子项才处理。\n\n%s\n\n请立即检查错误和已有证据，并明确选择：\n1. 若原方向仍值得继续，调用 coordinate_continue 为同一 Issue 创建新的 Execution；\n2. 若应更换方向，调用 coordinate_delegate 创建替代 Issue；\n3. 若失败不影响父目标，记录判断并继续父 Issue 的其他工作。", event.IssueID, message)
	}
	return message
}
