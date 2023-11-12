# Use a lightweight Linux distribution as the base image
FROM golang:1.21-alpine

RUN mkdir /app

RUN mkdir /app/public

# Copy the compiled Go application into the container
COPY signalrsrv /app/    
COPY signalRconfig.json /app/  
COPY public /app/public

# Set the working directory inside the container
WORKDIR /app

# Set permissions on the application (if needed)
RUN chmod +x signalrsrv

# Expose additional ports
EXPOSE 8222
# Define an entry point to run the application

CMD ["./signalrsrv"]