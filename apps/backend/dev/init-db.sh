#!/bin/bash
# Initialize the development database with seed data
# Usage: ./dev/init-db.sh [--reset]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_DIR="$SCRIPT_DIR"
DB_PATH="$DEV_DIR/kandev-dev.db"
SEED_SQL="$DEV_DIR/seed.sql"
SEED_SQL_PROCESSED="$DEV_DIR/.seed-processed.sql"

# Get the repository root path (three levels up from apps/backend/dev/)
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Kandev Development Database Initialization ===${NC}"
echo -e "Repository path: ${YELLOW}$REPO_ROOT${NC}"

# Check if reset flag is passed
if [[ "$1" == "--reset" ]]; then
    echo -e "${YELLOW}Resetting database...${NC}"
    rm -f "$DB_PATH" "$DB_PATH-wal" "$DB_PATH-shm"
fi

# Check if database already exists
if [[ -f "$DB_PATH" ]]; then
    echo -e "${YELLOW}Database already exists at: $DB_PATH${NC}"
    echo "Use --reset flag to recreate: ./dev/init-db.sh --reset"
    exit 0
fi

# Build the backend to ensure schema is created
echo -e "${GREEN}Building backend...${NC}"
cd "$SCRIPT_DIR/.."
go build -o /tmp/kandev-init ./cmd/kandev

# Create the database by running the backend briefly (to create schema)
echo -e "${GREEN}Creating database schema...${NC}"
KANDEV_DB_PATH="$DB_PATH" timeout 2 /tmp/kandev-init 2>/dev/null || true

# Wait for the database file to be created
sleep 1

if [[ ! -f "$DB_PATH" ]]; then
    echo -e "${RED}Failed to create database${NC}"
    exit 1
fi

# Process seed.sql - replace $KANDEV_REPO_PATH with actual path
echo -e "${GREEN}Processing seed data...${NC}"
sed "s|\\\$KANDEV_REPO_PATH|$REPO_ROOT|g" "$SEED_SQL" > "$SEED_SQL_PROCESSED"

# Apply seed data
echo -e "${GREEN}Applying seed data...${NC}"
sqlite3 "$DB_PATH" < "$SEED_SQL_PROCESSED"

# Clean up processed file
rm -f "$SEED_SQL_PROCESSED"

echo -e "${GREEN}=== Database initialized successfully ===${NC}"
echo -e "Database path: ${YELLOW}$DB_PATH${NC}"
echo ""
echo "To run the backend with this database:"
echo -e "  ${YELLOW}KANDEV_DB_PATH=$DB_PATH ./bin/kandev${NC}"
echo ""
echo "Or use the Makefile target:"
echo -e "  ${YELLOW}make dev${NC}"

