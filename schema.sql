-- Database Schema for ripit-be
-- MySQL 8.0+ Compatible
-- Primary Keys and Foreign Keys use UUID v4 (CHAR(36))

CREATE DATABASE IF NOT EXISTS `ripit_db` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `ripit_db`;

-- Drop tables in reverse foreign key order if re-creating
SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS `room_members`;
DROP TABLE IF EXISTS `tracks`;
DROP TABLE IF EXISTS `rooms`;
DROP TABLE IF EXISTS `users`;
SET FOREIGN_KEY_CHECKS = 1;

-- 1. Users Table
CREATE TABLE `users` (
    `id` CHAR(36) NOT NULL,
    `username` VARCHAR(255) NOT NULL,
    `avatar_url` VARCHAR(512) NULL DEFAULT NULL,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. Rooms Table
CREATE TABLE IF NOT EXISTS `rooms` (
    `id` CHAR(36) NOT NULL,
    `room_code` VARCHAR(16) NOT NULL,
    `name` VARCHAR(255) NOT NULL,
    `host_id` CHAR(36) NOT NULL,
    `current_track_id` CHAR(36) NULL DEFAULT NULL,
    `playback_state` ENUM('playing', 'paused') NOT NULL DEFAULT 'paused',
    `playback_position_ms` INT NOT NULL DEFAULT 0,
    `repeat_mode` ENUM('off', 'all', 'one') NOT NULL DEFAULT 'off',
    `is_shuffled` BOOLEAN NOT NULL DEFAULT FALSE,
    `last_sync_timestamp` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `session_started_at` TIMESTAMP NULL DEFAULT NULL,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_rooms_room_code` (`room_code`),
    KEY `idx_rooms_host_id` (`host_id`),
    CONSTRAINT `fk_rooms_host` FOREIGN KEY (`host_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. Room Members Table
CREATE TABLE `room_members` (
    `room_id` CHAR(36) NOT NULL,
    `user_id` CHAR(36) NOT NULL,
    `role` ENUM('host', 'listener') NOT NULL DEFAULT 'listener',
    `joined_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`room_id`, `user_id`),
    KEY `idx_room_members_user_id` (`user_id`),
    CONSTRAINT `fk_room_members_room` FOREIGN KEY (`room_id`) REFERENCES `rooms` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_room_members_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4. Tracks Table (Shared Queue)
CREATE TABLE `tracks` (
    `id` CHAR(36) NOT NULL,
    `room_id` CHAR(36) NOT NULL,
    `youtube_url` VARCHAR(512) NOT NULL,
    `title` VARCHAR(512) NOT NULL,
    `artist` VARCHAR(255) NOT NULL,
    `duration` VARCHAR(64) NOT NULL DEFAULT '0:00',
    `cover_url` VARCHAR(512) NOT NULL DEFAULT '',
    `lyrics` LONGTEXT NULL DEFAULT NULL,
    `has_lyrics` BOOLEAN NOT NULL DEFAULT FALSE,
    `added_by` CHAR(36) NOT NULL,
    `sort_order` INT NOT NULL DEFAULT 0,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_tracks_room_id` (`room_id`),
    KEY `idx_tracks_added_by` (`added_by`),
    KEY `idx_tracks_room_sort` (`room_id`, `sort_order`),
    CONSTRAINT `fk_tracks_room` FOREIGN KEY (`room_id`) REFERENCES `rooms` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_tracks_added_by` FOREIGN KEY (`added_by`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Add foreign key for current_track_id on rooms
ALTER TABLE `rooms`
    ADD CONSTRAINT `fk_rooms_current_track` FOREIGN KEY (`current_track_id`) REFERENCES `tracks` (`id`) ON DELETE SET NULL;
