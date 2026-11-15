#!/bin/sh
set -e

# Automatically run database migrations if DATABASE_URL is configured and RUN_MIGRATIONS != "false"
if [ -n "$DATABASE_URL" ] && [ "$RUN_MIGRATIONS" != "false" ]; then
  echo "Applying database migrations with goose..."
  /app/goose -dir /app/db/migrations postgres "$DATABASE_URL" up
fi

exec "$@"
