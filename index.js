const Discord = require("discord.js");
const client = new Discord.Client({
  intents: "60480",
});

const token = process.env["DISCORD_BOT_SECRET"];
console.log(token);

client.on("ready", () => {
  console.log("I'm in");
  console.log(client.user.username);
});

client.on("messageCreate", (msg) => {
  if (msg.author.id != client.user.id) {
    msg.channel.send(msg.content.split("").reverse().join(""));
  }
});

client.login(token);
