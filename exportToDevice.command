#! /bin/zsh

SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"
cd $SCRIPTPATH
pwd
./bin/extract ./config.yaml