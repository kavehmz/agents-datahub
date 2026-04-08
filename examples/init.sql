-- Create users table for testing
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Insert sample data
INSERT INTO users (id, name, email, status) VALUES
    ('user-001', 'John Doe', 'john@example.com', 'active'),
    ('user-002', 'Jane Smith', 'jane@example.com', 'active'),
    ('user-003', 'Bob Johnson', 'bob@example.com', 'inactive'),
    ('user-004', 'Alice Williams', 'alice@example.com', 'active'),
    ('user-005', 'Charlie Brown', 'charlie@example.com', 'active')
ON CONFLICT (id) DO NOTHING;