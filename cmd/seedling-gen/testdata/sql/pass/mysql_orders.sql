CREATE TABLE `accounts` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `code` VARCHAR(32) NOT NULL,
    `timezone_name` VARCHAR(64) NOT NULL DEFAULT ('UTC'),
    KEY `idx_accounts_code` (`code`)
) ENGINE=InnoDB;

CREATE TABLE `orders` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `account_id` BIGINT NOT NULL,
    `subtotal` DECIMAL(10,2) NOT NULL,
    `tax_amount` DECIMAL(10,2) NOT NULL DEFAULT (0.00),
    `gross_total` DECIMAL(10,2) GENERATED ALWAYS AS (`subtotal` + `tax_amount`) STORED,
    `external_ref` VARCHAR(64) NOT NULL,
    CONSTRAINT `orders_account_id_fk`
        FOREIGN KEY (`account_id`)
        REFERENCES `accounts` (`id`),
    CONSTRAINT `orders_external_ref_check`
        CHECK ((char_length(`external_ref`) > 6))
) ENGINE=InnoDB COMMENT='order rows';
