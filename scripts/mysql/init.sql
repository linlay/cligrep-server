-- Replace the placeholder password before running this script.
-- Restrict the host pattern and privileges for your environment as needed.
-- Run scripts/mysql/schema.sql next to create application tables.

CREATE DATABASE IF NOT EXISTS `cligrep`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

CREATE USER IF NOT EXISTS 'cligrep'@'%'
  IDENTIFIED WITH mysql_native_password BY 'replace-with-a-strong-password';

GRANT ALL PRIVILEGES ON `cligrep`.* TO 'cligrep'@'%';
FLUSH PRIVILEGES;
