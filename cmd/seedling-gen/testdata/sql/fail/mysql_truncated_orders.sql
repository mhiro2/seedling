CREATE TABLE `accounts` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `code` VARCHAR(32) NOT NULL
) ENGINE=InnoDB;

CREATE TABLE `orders` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `account_id` BIGINT NOT NULL,
    `gross_total` DECIMAL(10,2) GENERATED ALWAYS AS (`account_id` + 1) STORED,
    CONSTRAINT `orders_account_id_fk`
        FOREIGN KEY (`account_id`)
        REFERENCES `accounts` (`id`)
