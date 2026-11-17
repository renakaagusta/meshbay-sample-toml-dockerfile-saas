FROM node:22.21.1-alpine3.22
WORKDIR /app
ENV NODE_ENV=production
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force
COPY --chown=node:node worker ./worker
USER node
CMD ["node", "worker/src/worker.js"]
