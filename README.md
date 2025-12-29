# MissGirlyThing Discord Bot

A Discord bot built in Go that tracks messages, manages GIFs, and ranks users by offensive language.

## Features

- **MDR Detection**: Detects "mdr" variations (m d r, mdrrr, etc.) across last 3 messages and replies "tg"
- **xD Detection**: Replies "ca c'est un jolie smiley !" when someone says "xD"
- **Bot Mention**: Replies "UwU" when the bot is mentioned
- **GIF Management**: Assign GIFs to users (private commands). GIF displays for 3s when they message
- **Offensive Ranking**: Weekly leaderboard of offensive word usage (Sundays 8 PM)
- **Goodbye Message**: Says "aurevoir les amis, je vais dormir :3" when shutting down

## Quick Start

### With Docker (Recommended)

```bash
docker build -t missgirlything .
docker run -e DISCORD_TOKEN="your_token" \
           -e WORD_LIST="shit,fuck,damn" \
           -e GIF_DISPLAY_SECONDS=3 \
           -v $(pwd)/data:/app/data \
           missgirlything
```

### Without Docker

```bash
# Create .env file
cp .env.example .env
# Edit .env with your settings

# Run
go run main.go
```

## Environment Variables

- `DISCORD_TOKEN` - Your Discord bot token (required)
- `WORD_LIST` - Comma-separated offensive words (default: "shit,fuck,damn,ass,bitch,idiot,stupid")
- `GIF_DISPLAY_SECONDS` - GIF display duration in seconds (default: 3)

## Commands

- `/ping` - Test bot response
- `/setgif @user <gif_url>` - Set GIF for user (private, ephemeral)
- `/ranking` - Show weekly offensive ranking

## Discord Setup

1. Go to [Discord Developer Portal](https://discord.com/developers/applications)
2. Enable **Message Content Intent** in Bot settings
3. Invite with these permissions:
   - Send Messages
   - Read Messages/View Channels
   - Manage Messages
   - Use Slash Commands

## Data Storage

Data is stored in `save.json` (or `/app/data/save.json` in Docker) with user message history, GIFs, and offensive counts.

## Project Structure

```
missgirlything/
├── commands/     # Slash command handlers
├── services/     # Business logic (DB, messages, ranking)
├── types/        # Data structures
├── config/       # Configuration loader
├── main.go       # Entry point
└── Dockerfile    # Docker build
```
