-- 000001_init_schema.down.sql
ALTER TABLE `rooms` DROP FOREIGN KEY IF EXISTS `fk_rooms_current_track`;
DROP TABLE IF EXISTS `room_members`;
DROP TABLE IF EXISTS `tracks`;
DROP TABLE IF EXISTS `rooms`;
DROP TABLE IF EXISTS `users`;
