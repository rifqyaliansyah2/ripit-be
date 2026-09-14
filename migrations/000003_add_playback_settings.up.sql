ALTER TABLE `rooms`
    ADD COLUMN `repeat_mode` ENUM('off','all','one') NOT NULL DEFAULT 'off' AFTER `playback_position_ms`,
    ADD COLUMN `is_shuffled` BOOLEAN NOT NULL DEFAULT FALSE AFTER `repeat_mode`;