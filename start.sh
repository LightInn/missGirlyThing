#!/bin/bash
set -e

echo "🤖 MissGirlyThing Discord Bot"
echo "=============================="
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "⚠️  No .env file found. Creating from example..."
    cp .env.example .env
    echo "✅ Created .env file"
    echo "⚠️  Please edit .env and add your DISCORD_TOKEN"
    exit 1
fi

# Check if DISCORD_TOKEN is set
if ! grep -q "DISCORD_TOKEN=.*[^=]" .env; then
    echo "❌ DISCORD_TOKEN not set in .env file"
    echo "Please edit .env and add your Discord bot token"
    exit 1
fi

echo "🐳 Building Docker image..."
docker build -t missgirlything .

echo ""
echo "🚀 Starting bot..."
docker-compose up -d

echo ""
echo "✅ Bot is running!"
echo ""
echo "📋 Useful commands:"
echo "  docker-compose logs -f    # View logs"
echo "  docker-compose stop       # Stop bot"
echo "  docker-compose restart    # Restart bot"
echo "  docker-compose down       # Stop and remove container"
