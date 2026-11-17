FROM node:22.21.1-alpine3.22 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY web ./web
RUN npm run build

FROM node:22.21.1-alpine3.22 AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    PORT=3000
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --chown=node:node package.json package-lock.json ./
COPY --from=build --chown=node:node /app/web ./web
USER node
EXPOSE 3000
CMD ["npm", "run", "start"]
