FROM node:12-alpine as builder
WORKDIR /app
COPY ./frontend ./frontend
COPY ./nginx ./nginx
COPY ./certs ./certs
WORKDIR /app/frontend
RUN yarn install --quiet && yarn run build
# ENTRYPOINT ["tail", "-f", "/dev/null"]
FROM nginx:alpine
RUN rm -rf usr/share/nginx/html/*
COPY --from=builder app/frontend/build usr/share/nginx/html
COPY --from=builder app/nginx/nginx.conf etc/nginx/conf.d/default.conf
COPY --from=builder app/certs etc/nginx/ssl
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]