pipeline {
    agent any

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Run Go tests') {
            agent {
                docker {
                    image 'golang:1.24'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE"'
                sh 'go version'
                sh 'go test ./... -v'
            }
        }

        stage('Run golangci-lint') {
            agent {
                docker {
                    image 'golangci/golangci-lint:v1.64.8'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE"'
                sh 'golangci-lint version'
                sh 'golangci-lint run --timeout 5m'
            }
        }

        stage('Build Go binaries') {
            agent {
                docker {
                    image 'golang:1.24'
                    reuseNode true
                }
            }
            environment {
                HOME = "${WORKSPACE}"
                GOCACHE = "${WORKSPACE}/.gocache"
                GOMODCACHE = "${WORKSPACE}/.gomodcache"
            }
            steps {
                sh 'mkdir -p "$GOCACHE" "$GOMODCACHE" dist'

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/irc_collector ./cmd/irc_collector
                '''

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/oauth_server ./cmd/oauth_server
                '''

                sh '''
                    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
                    go build -o dist/kafka_consumer ./cmd/kafka_consumer
                '''

                sh 'ls -lah dist'
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'dist/**', allowEmptyArchive: true
        }
    }
}