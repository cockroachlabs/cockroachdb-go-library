-- Create the accounts table for the demo app
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    balance DECIMAL NOT NULL DEFAULT 0
);

-- Create vector table for AI demo (if using vector features)
-- CockroachDB native VECTOR type
CREATE TABLE IF NOT EXISTS vector_store (
    id TEXT PRIMARY KEY,
    content TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    embedding VECTOR(3)
);

-- Seed some demo data
INSERT INTO accounts (name, balance) VALUES
    ('Alice', 1000.00),
    ('Bob', 500.00),
    ('Charlie', 750.00)
ON CONFLICT DO NOTHING;
