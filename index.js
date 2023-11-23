const dotenv = require("dotenv");

dotenv.config();
// get token from dotenv
const token = process.env.DISCORD_BOT_SECRET;
console.log(token);

const fs = require("fs");
const {
  Client,
  Collection,
  GatewayIntentBits,
  Partials,
  REST,
  Routes,
} = require("discord.js");

/**
 * From v13, specifying the intents is compulsory.
 * @type {import('./typings').Client}
 * @description Main Application Client */

// @ts-ignore
const client = new Client({
  // Please add all intents you need, more detailed information @ https://ziad87.net/intents/
  intents: [
    GatewayIntentBits.Guilds,
    GatewayIntentBits.DirectMessages,
    GatewayIntentBits.GuildMessages,
    GatewayIntentBits.MessageContent,
  ],
  partials: [Partials.Channel],
});

client.login(token);
