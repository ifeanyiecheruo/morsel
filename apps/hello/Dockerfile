FROM node:22-alpine AS build
WORKDIR /app
COPY package.json tsconfig.json ./
RUN npm install
COPY server.mts ./
RUN npx tsc

FROM node:22-alpine
WORKDIR /app
COPY --from=build /app/dist/ ./
EXPOSE 3000
CMD ["node", "server.mjs"]
