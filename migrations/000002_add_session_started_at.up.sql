ALTER TABLE `rooms`
    ADD COLUMN `session_started_at` TIMESTAMP NULL DEFAULT NULL AFTER `last_sync_timestamp`;