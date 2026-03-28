package i18n

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/linlay/cligrep-server/internal/models"
)

var catalogs = map[string]map[string]string{
	"en": {
		"method_not_allowed":                 "method not allowed",
		"invalid_json_body":                  "invalid json body",
		"missing_cli_slug":                   "missing cli slug",
		"not_found":                          "not found",
		"failed_init_auth_state":             "failed to initialize auth state",
		"cli_slug_required":                  "cliSlug is required",
		"valid_user_id_required":             "valid userId is required",
		"command_line_empty":                 "command line cannot be empty",
		"multiline_not_allowed":              "multiline input is not allowed",
		"shell_operators_disabled":           "shell operators, pipes, and redirects are disabled in v1",
		"builtin_must_use_builtin_exec":      "builtin commands must use /api/v1/builtin/exec",
		"cli_reference_only":                 "this CLI is indexed for reference only and cannot be executed in the sandbox",
		"unauthorized":                       "unauthorized",
		"auth_not_configured":                "auth is not configured",
		"invalid_credentials":                "invalid username or password",
		"username_taken":                     "username is already taken",
		"invalid_username":                   "username must match [a-zA-Z0-9_.-]{3,32}",
		"weak_password":                      "password must be at least 8 characters",
		"invalid_display_name":               "display name cannot be empty",
		"builtin_help_loaded":                "Built-in command reference loaded.",
		"builtin_clear_done":                 "Terminal cleared. Back to the registry homepage.",
		"builtin_unknown_command":            "Unknown built-in command %q.",
		"builtin_unknown_command_stderr":     "unknown built-in command: %s",
		"builtin_available_hint":             "Available built-ins: grep, create, make, help, clear.",
		"builtin_clear_hint_search":          "Use grep <query> to search.",
		"builtin_clear_hint_open":            "Press Enter on a highlighted result to open it.",
		"builtin_grep_query_required":        "grep needs a query. Example: grep ripgrep",
		"builtin_grep_query_hint":            "Search name, tags, summaries, and stored help text.",
		"builtin_grep_found":                 "Found %d CLI matches for %q.",
		"builtin_grep_hint_open":             "Enter on a highlighted result opens its run mode.",
		"builtin_grep_hint_escape":           "Esc returns to the homepage grid.",
		"builtin_create_usage":               "Usage: create python \"build a CLI that ...\"",
		"builtin_create_usage_stdout":        "Use create python \"your spec\" to generate and preview a Python CLI.",
		"builtin_create_done":                "Generated %s from spec %q and previewed it in the Python sandbox.",
		"builtin_create_hint_saved":          "The generated file is saved to the database as an asset.",
		"builtin_create_hint_next":           "Use make dockerfile <cli> for a packaging draft next.",
		"builtin_make_usage":                 "Usage: make sandbox <cli> or make dockerfile <cli>",
		"builtin_make_usage_stdout":          "Examples:\nmake sandbox grep\nmake dockerfile grep",
		"builtin_make_unknown_target":        "Unknown make target %q.",
		"builtin_make_unknown_target_stderr": "unknown make target: %s",
		"builtin_make_done":                  "Generated %s preview for %s.",
		"builtin_make_hint_preview":          "Generated artifacts are previews backed by the database.",
		"builtin_make_hint_escape":           "Use Esc to return to search/home.",
		"builtin_help_title":                 "CLI Grep built-ins",
		"builtin_help_grep_desc":             "Search indexed CLIs by name, summary, tags, and stored help text.",
		"builtin_help_create_desc":           "Generate a single-file Python CLI scaffold and preview it with --help.",
		"builtin_help_make_sandbox_desc":     "Generate a sandbox recipe preview.",
		"builtin_help_make_dockerfile_desc":  "Generate a Dockerfile preview for the selected CLI.",
		"builtin_help_footer":                "clear / help",
		"builtin_generic_hint_search_wrap":   "Search mode auto-wraps plain text as grep <query>.",
		"builtin_generic_hint_runtime":       "Built-ins stay website-native; ordinary CLIs run in Docker sandboxes.",
	},
	"zh": {
		"method_not_allowed":                 "不支持该请求方法",
		"invalid_json_body":                  "JSON 请求体无效",
		"missing_cli_slug":                   "缺少 cli slug",
		"not_found":                          "未找到资源",
		"failed_init_auth_state":             "初始化登录状态失败",
		"cli_slug_required":                  "必须提供 cliSlug",
		"valid_user_id_required":             "必须提供有效的 userId",
		"command_line_empty":                 "命令行不能为空",
		"multiline_not_allowed":              "不允许多行输入",
		"shell_operators_disabled":           "v1 版本禁用了 shell 运算符、管道和重定向",
		"builtin_must_use_builtin_exec":      "内置命令必须使用 /api/v1/builtin/exec",
		"cli_reference_only":                 "这个 CLI 仅供查阅，当前不能在沙箱中执行",
		"unauthorized":                       "未登录",
		"auth_not_configured":                "认证功能尚未配置",
		"invalid_credentials":                "用户名或密码错误",
		"username_taken":                     "用户名已被占用",
		"invalid_username":                   "用户名必须匹配 [a-zA-Z0-9_.-]{3,32}",
		"weak_password":                      "密码至少需要 8 个字符",
		"invalid_display_name":               "显示名称不能为空",
		"builtin_help_loaded":                "已加载内置命令说明。",
		"builtin_clear_done":                 "终端已清空，已返回目录首页。",
		"builtin_unknown_command":            "未知的内置命令 %q。",
		"builtin_unknown_command_stderr":     "未知的内置命令：%s",
		"builtin_available_hint":             "当前可用的内置命令：grep、create、make、help、clear。",
		"builtin_clear_hint_search":          "使用 grep <query> 搜索。",
		"builtin_clear_hint_open":            "选中结果后按回车即可打开。",
		"builtin_grep_query_required":        "grep 需要一个查询词。例如：grep ripgrep",
		"builtin_grep_query_hint":            "会搜索名称、标签、摘要和已存储的帮助文本。",
		"builtin_grep_found":                 "找到 %d 个与 %q 匹配的 CLI。",
		"builtin_grep_hint_open":             "在高亮结果上按回车可进入执行模式。",
		"builtin_grep_hint_escape":           "按 Esc 返回首页网格。",
		"builtin_create_usage":               "用法：create python \"build a CLI that ...\"",
		"builtin_create_usage_stdout":        "使用 create python \"your spec\" 来生成并预览一个 Python CLI。",
		"builtin_create_done":                "已生成 %s，规格为 %q，并已在 Python 沙箱中完成预览。",
		"builtin_create_hint_saved":          "生成的文件已作为资产保存到数据库。",
		"builtin_create_hint_next":           "接下来可以用 make dockerfile <cli> 生成打包草稿。",
		"builtin_make_usage":                 "用法：make sandbox <cli> 或 make dockerfile <cli>",
		"builtin_make_usage_stdout":          "示例：\nmake sandbox grep\nmake dockerfile grep",
		"builtin_make_unknown_target":        "未知的 make 目标 %q。",
		"builtin_make_unknown_target_stderr": "未知的 make 目标：%s",
		"builtin_make_done":                  "已生成 %s 预览，目标 CLI 为 %s。",
		"builtin_make_hint_preview":          "这些生成结果是由数据库支撑的预览资产。",
		"builtin_make_hint_escape":           "按 Esc 可返回搜索页或首页。",
		"builtin_help_title":                 "CLI Grep 内置命令",
		"builtin_help_grep_desc":             "按名称、摘要、标签和帮助文本搜索已索引的 CLI。",
		"builtin_help_create_desc":           "生成单文件 Python CLI 脚手架，并用 --help 预览。",
		"builtin_help_make_sandbox_desc":     "生成沙箱配置预览。",
		"builtin_help_make_dockerfile_desc":  "为当前 CLI 生成 Dockerfile 预览。",
		"builtin_help_footer":                "clear / help",
		"builtin_generic_hint_search_wrap":   "搜索模式会自动把纯文本包装成 grep <query>。",
		"builtin_generic_hint_runtime":       "内置命令在站内执行，普通 CLI 在 Docker 沙箱中运行。",
	},
}

func Text(ctx context.Context, key string, args ...any) string {
	lang := MessageLanguage(LocaleFromContext(ctx))
	catalog := catalogs[lang]
	if catalog == nil {
		catalog = catalogs["en"]
	}
	template := catalog[key]
	if template == "" {
		template = catalogs["en"][key]
	}
	if template == "" {
		template = key
	}
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}

func LocalizeError(ctx context.Context, err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Text(ctx, "not_found")
	case errors.Is(err, models.ErrUnauthorized):
		return Text(ctx, "unauthorized")
	case errors.Is(err, models.ErrAuthNotConfigured):
		return Text(ctx, "auth_not_configured")
	case errors.Is(err, models.ErrInvalidCredentials):
		return Text(ctx, "invalid_credentials")
	case errors.Is(err, models.ErrUsernameTaken):
		return Text(ctx, "username_taken")
	case errors.Is(err, models.ErrInvalidUsername):
		return Text(ctx, "invalid_username")
	case errors.Is(err, models.ErrWeakPassword):
		return Text(ctx, "weak_password")
	case errors.Is(err, models.ErrInvalidDisplayName):
		return Text(ctx, "invalid_display_name")
	}

	switch strings.TrimSpace(err.Error()) {
	case "command line cannot be empty":
		return Text(ctx, "command_line_empty")
	case "multiline input is not allowed":
		return Text(ctx, "multiline_not_allowed")
	case "shell operators, pipes, and redirects are disabled in v1":
		return Text(ctx, "shell_operators_disabled")
	case "builtin commands must use /api/v1/builtin/exec":
		return Text(ctx, "builtin_must_use_builtin_exec")
	case "this CLI is indexed for reference only and cannot be executed in the sandbox":
		return Text(ctx, "cli_reference_only")
	case "valid userId is required":
		return Text(ctx, "valid_user_id_required")
	case "cliSlug is required":
		return Text(ctx, "cli_slug_required")
	default:
		return err.Error()
	}
}
