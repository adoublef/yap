CREATE TABLE yaps (
    /* xid formatted */
    'id' TEXT,
    /* min 1 character, max 240 characters */
    'content' TEXT NOT NULL,
    /* TODO add region */
    'region' TEXT NOT NULL DEFAULT 'lhr'
    --
    CONSTRAINT yap_content CHECK (content <> '' AND length(content) <= 240),
    PRIMARY KEY (id)
);

/* yaps upvotes, downvotes, json events */