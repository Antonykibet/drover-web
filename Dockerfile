FROM nginx:alpine
COPY index.html /usr/share/nginx/html/
COPY shots/ /usr/share/nginx/html/shots/
