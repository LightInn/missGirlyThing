const {Client} = require('discord.js');

// Integer des intents calculé en fonction des intents nécessaires
const intents = 687195286592;

const client = new Client({intents});


// Token du bot - Remplace 'TON_TOKEN_ICI' par le token de ton bot
const token = 'NjI3ODY2MDg0MDc4MjU2MTM0.GHR9Z9.1qgsQlQBuORXOEHsp7-8afnMoYpx0BWGWuf4eE';

// Dictionnaire pour stocker les préférences d'emoji
let emojiPreferences = {};

client.on('ready', () => {
    console.log(`Connecté en tant que ${client.user.tag}!`);
});


client.on('message', message => {
    // Ignorer les messages du bot lui-même
    if (message.author.bot) return;

    // Répondre avec l'emoji personnalisé
    if (emojiPreferences[message.author.id]) {
        message.react(emojiPreferences[message.author.id]);
    }

    // Commande pour définir l'emoji - Exemple: "!setemoji :smile:"
    if (message.content.startsWith('!setemoji')) {
        const args = message.content.split(' ');
        if (args.length === 2) {
            emojiPreferences[message.author.id] = args[1];
            message.channel.send(`Emoji pour ${message.author.username} défini sur ${args[1]}`);
        }
    }
});

client.login(token);
