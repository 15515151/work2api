package trae

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"work2api/internal/core/provider"
	"work2api/internal/trae/auth"
	"work2api/internal/trae/upstream"
)

func retryable(kind upstream.ErrKind) bool {
	return kind == upstream.ErrSoftRate || kind == upstream.ErrServer || kind == upstream.ErrSessionDead || kind == upstream.ErrPlanLimit
}

func retryAfter(value string, now time.Time) time.Duration {
	var delay time.Duration
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		if seconds > 86400 {
			seconds = 86400
		}
		delay = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		delay = at.Sub(now)
	}
	if delay < 0 {
		return 0
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	return delay
}

// Return a useful classification and numeric code, without echoing an upstream
// body that may contain credentials, signed links, or fragments of user input.
func rejectionError(kind upstream.ErrKind, status int, code int64) error {
	message, errorType := "Trae 上游拒绝请求", "upstream_error"
	switch kind {
	case upstream.ErrInputTooLarge:
		status, message, errorType = 400, "Trae 上游拒绝过大的输入，请压缩上下文或缩小请求", "context_length_exceeded"
	case upstream.ErrModelUnavailable:
		status, message, errorType = 400, "该模型不在当前 Trae 产品聊天通道可用，请刷新模型目录并选择可用模型", "model_unavailable"
	case upstream.ErrPlanLimit:
		status, message, errorType = 429, "Trae 账号的积分或模型配额不足，请等待额度恢复或使用其他有权账号", "quota_exceeded"
	case upstream.ErrSoftRate:
		status, message, errorType = 429, "Trae 上游限流，请稍后重试", "rate_limit_exceeded"
	case upstream.ErrSessionDead:
		status, message, errorType = 503, "Trae 账号授权已失效，自动刷新未恢复，请重新授权", "account_unavailable"
	case upstream.ErrServer:
		status, message = 502, "Trae 上游服务暂不可用，请稍后重试"
	}
	if status < 400 || status > 599 {
		status = 502
	}
	if code != 0 {
		message += fmt.Sprintf("（上游 code=%d）", code)
	}
	return provider.NewAPIError(status, message, errorType)
}

func (r *Runtime) forceRefresh(ctx context.Context, id string, identity *auth.Auth) error {
	if accountID(identity.UID) != id {
		return errors.New("Trae 账号授权已变更")
	}
	return r.refreshCredentials(ctx, identity, true)
}
