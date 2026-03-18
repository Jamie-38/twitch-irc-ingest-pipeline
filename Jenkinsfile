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
    }
}