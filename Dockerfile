# syntax=docker/dockerfile:1

# No build step: the page is a single static index.html plus its images.
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html /usr/share/nginx/html/index.html
COPY shots/ /usr/share/nginx/html/shots/
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
