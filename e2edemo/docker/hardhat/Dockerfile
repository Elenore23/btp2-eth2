FROM node:20.15.1-alpine3.20

RUN mkdir -p /app
WORKDIR /app

RUN npm install --save-dev hardhat
ADD hardhat.config.js /app
