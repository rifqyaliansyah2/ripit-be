ALTER TABLE `tracks` DROP FOREIGN KEY `fk_tracks_added_by`;
ALTER TABLE `tracks` MODIFY `added_by` CHAR(36) NULL DEFAULT NULL;
ALTER TABLE `tracks`
    ADD CONSTRAINT `fk_tracks_added_by` FOREIGN KEY (`added_by`) REFERENCES `users` (`id`) ON DELETE SET NULL;