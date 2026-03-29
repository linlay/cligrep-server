USE `cligrep`;

INSERT INTO cli_locale_content (
  CLI_SLUG_,
  LOCALE_,
  DISPLAY_NAME_,
  SUMMARY_,
  HELP_TEXT_,
  TAGS_JSON_,
  CREATED_AT_,
  UPDATED_AT_
) VALUES
  ('grep', 'zh', 'grep', '在沙箱内使用正则表达式搜索文本。', '在沙箱环境中使用正则表达式搜索文本。示例：grep --help。', '["搜索","busybox","文本"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('find', 'zh', 'find', '在沙箱运行时遍历目录树并匹配文件。', '在沙箱运行时遍历目录树并匹配文件。示例：find --help。', '["搜索","文件系统","busybox"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('sed', 'zh', 'sed', '按行流式编辑文本内容。', '按行流式编辑文本内容。示例：sed --help。', '["转换","busybox","文本"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('awk', 'zh', 'awk', '面向结构化文本的模式扫描与报表生成工具。', '面向结构化文本的模式扫描与报表生成工具。示例：awk --help。', '["报表","busybox","文本"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('ls', 'zh', 'ls', '在隔离容器中列出文件并查看目录。', '在隔离容器中列出文件并查看目录。示例：ls --help。', '["列表","文件系统","busybox"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('sort', 'zh', 'sort', '使用标准 shell 风格参数对文本输入排序。', '使用标准 shell 风格参数对文本输入排序。示例：sort --help。', '["排序","busybox","文本"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('builtin-grep', 'zh', '内置 grep', '站内原生命令，用于搜索 CLI 目录。', '使用 grep <query> 搜索已索引的 CLI。', '["内置","搜索","核心"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('builtin-create', 'zh', '内置 create', '在网站流程中生成 Python CLI 脚手架。', '使用 create python "your spec"。', '["内置","生成器","Python"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000'),
  ('builtin-make', 'zh', '内置 make', '为 CLI 生成 Dockerfile 和沙箱配置预览。', '使用 make sandbox <cli> 或 make dockerfile <cli>。', '["内置","Docker","沙箱"]', '2026-03-28 00:00:00.000', '2026-03-28 00:00:00.000')
ON DUPLICATE KEY UPDATE
  DISPLAY_NAME_ = VALUES(DISPLAY_NAME_),
  SUMMARY_ = VALUES(SUMMARY_),
  HELP_TEXT_ = VALUES(HELP_TEXT_),
  TAGS_JSON_ = VALUES(TAGS_JSON_),
  UPDATED_AT_ = VALUES(UPDATED_AT_);
