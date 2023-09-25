-- Copyright 2023 Google LLC
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

USE chat;

CREATE TABLE IF NOT EXISTS threads (
    thread  INTEGER AUTO_INCREMENT PRIMARY KEY,
    creator VARCHAR(256) NOT NULL
);

CREATE TABLE IF NOT EXISTS userthreads (
    user   VARCHAR(256) NOT NULL,
    thread INTEGER NOT NULL,
    CONSTRAINT uthread FOREIGN KEY(thread) REFERENCES threads(thread)
);

CREATE TABLE IF NOT EXISTS images (
    id    INTEGER AUTO_INCREMENT PRIMARY KEY,
    image BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS posts (
    post    INTEGER AUTO_INCREMENT PRIMARY KEY,
    thread  INTEGER NOT NULL,
    creator VARCHAR(256) NOT NULL,
    time    INTEGER NOT NULL,
    text    TEXT NOT NULL,
    imageid INTEGER,
    CONSTRAINT pthread FOREIGN KEY (thread) REFERENCES threads(thread),
    CONSTRAINT pimageid FOREIGN KEY (imageid) REFERENCES images(id)
);

CREATE INDEX thread ON threads (thread);
CREATE INDEX uthread ON userthreads (thread);
CREATE INDEX image ON images (id);
