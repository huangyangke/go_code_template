CREATE TABLE IF NOT EXISTS articles (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    title      VARCHAR(255)    NOT NULL,
    content    TEXT            NOT NULL,
    author     VARCHAR(100)    NOT NULL,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME        NULL,
    PRIMARY KEY (id),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
