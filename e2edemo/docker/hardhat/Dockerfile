FROM node:22.15.1-alpine3.21

RUN mkdir -p /app
WORKDIR /app

RUN npm install --save-dev hardhat
ADD hardhat.config.js /app
