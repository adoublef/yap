CREATE TABLE yaps (
    /* xid formatted */
    'id' TEXT,
    'content' TEXT NOT NULL,
    'region' TEXT NOT NULL DEFAULT 'lhr'
    --
    CONSTRAINT valid_content CHECK (content <> '' AND length(content) <= 240),
    CONSTRAINT fly_region CHECK (region IN ('lhr', 'syd', 'iad')),
    PRIMARY KEY (id)
);

/* yaps upvotes, downvotes, json events */
CREATE TABLE votes (
    /* https://stackoverflow.com/questions/7905859/is-there-auto-increment-in-sqlite */
    'id' INT,
    'yap' TEXT NOT NULL,
    'score' INT NOT NULL DEFAULT 1,
    --
    CONSTRAINT is_score CHECK (score IN (0, 1)),
    FOREIGN KEY (yap) REFERENCES yaps (id),
    PRIMARY KEY (id)
);