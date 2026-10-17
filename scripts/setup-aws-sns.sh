#!/bin/bash
################################################################################
# AWS SNS SMS Setup Script
# 
# This script sets up AWS SNS for production SMS notifications. It will:
# 1. Verify AWS CLI credentials work (from ~/.aws/credentials or AWS CLI login)
# 2. Set SMS preferences for production use
# 3. Optionally create an SNS topic for alarm notifications
# 4. Configure spending limits and origination numbers
#    (as implemented: sets MonthlySpendLimit only; no origination numbers)
# 5. Test SMS delivery
# 6. Update .env with the application runtime user credentials
#    (as implemented: only AWS_SNS_TOPIC_ARN is written to .env, and only when
#    a topic is created; credentials are never written - add them yourself)
#
# Actual order: load AWS_REGION from .env, check the CLI, verify credentials,
# check SNS access, show current SMS attributes, set SMS type and monthly
# spend limit (step 2/4), print limit-increase instructions, optionally create
# a topic and subscribe phones (step 3), optionally send a test (step 5), and
# print a summary. Every choice is prompted on stdin, so run it interactively.
# These are real, account-wide AWS changes and the test SMS is billed.
#
# Usage: ./scripts/setup-aws-sns.sh    (from any directory; no arguments)
#
# Prerequisites:
# - AWS CLI installed (brew install awscli or apt-get install awscli)
# - AWS CLI configured with admin credentials (aws configure)
# - .env file with AWS_REGION (or will prompt for region)
#
# Note: This script uses YOUR admin AWS CLI credentials to SET UP the SNS system.
#       The .env file contains the APPLICATION's runtime credentials (different user).
################################################################################

set -e  # Exit on error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$PROJECT_ROOT/.env"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

#######################################
# Function to print colored output
# Print a message prefixed with a blue "ℹ" marker.
# Globals:
#   BLUE, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the message to stdout.
# Returns:
#   0.
#######################################
print_info() { echo -e "${BLUE}ℹ ${NC}$1"; }
#######################################
# Print a message prefixed with a green "✓" marker.
# Globals:
#   GREEN, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the message to stdout.
# Returns:
#   0.
#######################################
print_success() { echo -e "${GREEN}✓${NC} $1"; }
#######################################
# Print a message prefixed with a yellow "⚠" marker.
# Globals:
#   YELLOW, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the message to stdout.
# Returns:
#   0.
#######################################
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
#######################################
# Print a message prefixed with a red "✗" marker.
# Globals:
#   RED, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the message to stdout.
# Returns:
#   0.
#######################################
print_error() { echo -e "${RED}✗${NC} $1"; }

#######################################
# Function to load .env file for region info only
# Reads AWS_REGION (only) from the project .env file, if present.
# Credentials in .env are deliberately not loaded: setup runs with the admin
# CLI credentials. Quotes and spaces are stripped from the value.
# Globals:
#   ENV_FILE (read). AWS_REGION (set when .env defines it).
# Arguments:
#   None
# Outputs:
#   Writes status messages to stdout.
# Returns:
#   0.
#######################################
load_env() {
    if [ ! -f "$ENV_FILE" ]; then
        print_warning ".env file not found at: $ENV_FILE"
        print_info "Will create .env file during setup"
        return 0
    fi
    
    print_info "Loading region from .env file..."
    
    # Load AWS region from .env (but NOT credentials - we use CLI credentials)
    local env_region=$(grep -E "^AWS_REGION=" "$ENV_FILE" | cut -d'=' -f2 | tr -d ' "')
    
    if [ -n "$env_region" ]; then
        AWS_REGION="$env_region"
        print_success "Using region from .env: $AWS_REGION"
    else
        print_info "No AWS_REGION in .env, will prompt during setup"
    fi
}

#######################################
# Function to check AWS CLI installation
# Verifies that the aws CLI is on PATH and reports its version.
# Globals:
#   None
# Arguments:
#   None
# Outputs:
#   Writes status messages to stdout.
# Returns:
#   0; exits 1 if aws is not installed.
#######################################
check_aws_cli() {
    print_info "Checking AWS CLI installation..."
    if ! command -v aws &> /dev/null; then
        print_error "AWS CLI not found"
        print_info "Install with: brew install awscli (macOS) or apt-get install awscli (Linux)"
        exit 1
    fi
    
    local version=$(aws --version 2>&1 | cut -d' ' -f1 | cut -d'/' -f2)
    print_success "AWS CLI installed (version $version)"
}

#######################################
# Function to verify AWS CLI credentials
# Checks the CLI credentials with `aws sts get-caller-identity`.
# Prints the account ID and caller ARN, then prompts for a region if
# AWS_REGION is still empty (from neither the environment nor .env).
# Globals:
#   AWS_REGION (read; set from the prompt when empty).
# Arguments:
#   None
# Outputs:
#   Writes account details and the prompt to stdout; reads stdin.
# Returns:
#   0; exits 1 if authentication fails or no region is entered.
#######################################
verify_credentials() {
    print_info "Verifying AWS CLI credentials..."
    print_warning "NOTE: Using your AWS CLI credentials (from ~/.aws/credentials or aws configure)"
    print_warning "      This is DIFFERENT from the application runtime credentials in .env"
    
    if aws sts get-caller-identity &> /dev/null; then
        local account=$(aws sts get-caller-identity --query Account --output text)
        local user=$(aws sts get-caller-identity --query Arn --output text)
        print_success "AWS CLI credentials valid"
        print_info "Account: $account"
        print_info "Admin User: $user"
    else
        print_error "Failed to authenticate with AWS CLI"
        print_info "Run: aws configure"
        print_info "Or ensure ~/.aws/credentials contains valid credentials"
        exit 1
    fi
    
    # Prompt for region if not set
    if [ -z "$AWS_REGION" ]; then
        echo ""
        read -p "Enter AWS region (e.g., us-west-2, us-east-1): " AWS_REGION
        if [ -z "$AWS_REGION" ]; then
            print_error "Region is required"
            exit 1
        fi
    fi
}

#######################################
# Function to check SNS permissions
# Verifies SNS access by calling `aws sns list-topics` in AWS_REGION.
# Globals:
#   AWS_REGION (read).
# Arguments:
#   None
# Outputs:
#   Writes status messages to stdout.
# Returns:
#   0; exits 1 if the call fails.
#######################################
check_permissions() {
    print_info "Checking SNS permissions..."
    
    if aws sns list-topics --region "$AWS_REGION" &> /dev/null; then
        print_success "SNS permissions verified"
    else
        print_error "Insufficient SNS permissions"
        print_info "Ensure IAM user has AmazonSNSFullAccess or sns:* permissions"
        exit 1
    fi
}

#######################################
# Function to get current SMS attributes
# Prints the account's current SNS SMS attributes as a table.
# Globals:
#   AWS_REGION (read).
# Arguments:
#   None
# Outputs:
#   Writes the attribute table to stdout.
# Returns:
#   0 on success; 1 if the AWS call fails. main calls this as a plain
#   statement under `set -e`, so a failure ends the whole script here.
#######################################
get_sms_attributes() {
    print_info "Fetching current SMS attributes..."
    
    echo ""
    echo "Current SMS Configuration:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    aws sns get-sms-attributes --region "$AWS_REGION" --output table 2>/dev/null || {
        print_warning "No SMS attributes set yet"
        return 1
    }
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

#######################################
# Function to configure SMS for production
# Prompts for and sets DefaultSMSType and MonthlySpendLimit account-wide.
# Choice "1" selects Promotional; anything else (default) Transactional. The
# spending limit defaults to 10 and is passed to AWS unvalidated. Both are
# account-level settings for the region, not specific to this application.
# Globals:
#   AWS_REGION (read). SMS_TYPE (set).
# Arguments:
#   None
# Outputs:
#   Writes prompts and status to stdout; reads stdin.
# Returns:
#   0; exits non-zero (via `set -e`) if an aws call fails, e.g. when the
#   limit exceeds the account's approved quota.
#######################################
configure_production_sms() {
    print_info "Configuring SMS for production use..."
    
    echo ""
    echo "SMS Type Options:"
    echo "  1. Promotional - Lower cost, lower priority, lower deliverability"
    echo "  2. Transactional - Higher cost, higher priority, better deliverability (RECOMMENDED for alarms)"
    echo ""
    read -p "Select SMS type (1 or 2) [default: 2]: " sms_type_choice
    sms_type_choice=${sms_type_choice:-2}
    
    if [ "$sms_type_choice" = "1" ]; then
        SMS_TYPE="Promotional"
    else
        SMS_TYPE="Transactional"
    fi
    
    print_info "Setting SMS type to: $SMS_TYPE"
    
    aws sns set-sms-attributes \
        --attributes "DefaultSMSType=$SMS_TYPE" \
        --region "$AWS_REGION"
    
    print_success "SMS type configured"
    
    # Set spending limit
    echo ""
    echo "Current AWS SNS default spending limit is \$1.00/month (sandbox mode)"
    echo "For production, you should request a spending limit increase."
    echo ""
    read -p "Set monthly spending limit in USD [default: 10]: " spending_limit
    spending_limit=${spending_limit:-10}
    
    print_info "Setting monthly spending limit to \$$spending_limit..."
    
    aws sns set-sms-attributes \
        --attributes "MonthlySpendLimit=$spending_limit" \
        --region "$AWS_REGION"
    
    print_success "Spending limit set to \$$spending_limit/month"
    print_warning "Note: You may need to request a limit increase through AWS Support"
    print_info "Visit: https://console.aws.amazon.com/support/home#/case/create?issueType=service-limit-increase"
}

#######################################
# Function to request spending limit increase
# Prints manual steps for requesting a higher SMS spending limit.
# Globals:
#   AWS_REGION (read).
# Arguments:
#   None
# Outputs:
#   Writes instructions to stdout and waits for Enter on stdin.
# Returns:
#   0.
#######################################
request_limit_increase() {
    echo ""
    print_info "To increase your SMS spending limit beyond \$1/month:"
    echo ""
    echo "1. Sign in to AWS Console: https://console.aws.amazon.com/"
    echo "2. Go to: Support > Create Case"
    echo "3. Select: Service Limit Increase"
    echo "4. Limit Type: SNS Text Messaging"
    echo "5. Request details:"
    echo "   - Region: $AWS_REGION"
    echo "   - Resource Type: General Limits"
    echo "   - Limit: Account Spending Limit"
    echo "   - New Limit: [your desired monthly limit]"
    echo "6. Use Case Description:"
    echo "   'Weather alarm notifications for Tempest weather station monitoring system."
    echo "    Critical alerts for lightning, temperature, wind, and severe weather.'"
    echo ""
    read -p "Press Enter to continue..."
}

#######################################
# Function to create SNS topic
# Optionally creates an SNS topic, records it in .env and subscribes phones.
# Each step is confirmed interactively (default "y"; only a literal "y"
# counts as yes). The topic ARN replaces an existing AWS_SNS_TOPIC_ARN line
# in .env or is appended (creating .env if it does not exist). Each phone
# number entered is subscribed with protocol sms until an empty line.
# Globals:
#   AWS_REGION, ENV_FILE, OSTYPE (read). TOPIC_ARN, create_topic, topic_name,
#   update_env, subscribe_phones, phone_number (set; not local).
# Arguments:
#   None
# Outputs:
#   Writes prompts and status to stdout; reads stdin; may modify .env.
# Returns:
#   0; exits non-zero (via `set -e`) if an aws call fails.
#######################################
create_topic() {
    echo ""
    read -p "Create an SNS topic for alarm notifications? (y/n) [default: y]: " create_topic
    create_topic=${create_topic:-y}
    
    if [ "$create_topic" != "y" ]; then
        print_info "Skipping topic creation"
        return 0
    fi
    
    echo ""
    read -p "Enter topic name [default: tempest-weather-alarms]: " topic_name
    topic_name=${topic_name:-tempest-weather-alarms}
    
    print_info "Creating SNS topic: $topic_name..."
    
    TOPIC_ARN=$(aws sns create-topic \
        --name "$topic_name" \
        --region "$AWS_REGION" \
        --output text)
    
    print_success "Topic created: $TOPIC_ARN"
    
    # Add display name
    aws sns set-topic-attributes \
        --topic-arn "$TOPIC_ARN" \
        --attribute-name DisplayName \
        --attribute-value "Tempest Alarms" \
        --region "$AWS_REGION"
    
    print_success "Topic display name set"
    
    # Update .env file
    echo ""
    read -p "Update .env file with this topic ARN? (y/n) [default: y]: " update_env
    update_env=${update_env:-y}
    
    if [ "$update_env" = "y" ]; then
        # Check if AWS_SNS_TOPIC_ARN exists in .env
        if grep -q "^AWS_SNS_TOPIC_ARN=" "$ENV_FILE"; then
            # Update existing line (macOS and Linux compatible)
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' "s|^AWS_SNS_TOPIC_ARN=.*|AWS_SNS_TOPIC_ARN=$TOPIC_ARN|" "$ENV_FILE"
            else
                sed -i "s|^AWS_SNS_TOPIC_ARN=.*|AWS_SNS_TOPIC_ARN=$TOPIC_ARN|" "$ENV_FILE"
            fi
        else
            # Add new line
            echo "AWS_SNS_TOPIC_ARN=$TOPIC_ARN" >> "$ENV_FILE"
        fi
        print_success "Updated .env file with topic ARN"
    fi
    
    # Subscribe phone numbers
    echo ""
    read -p "Subscribe phone numbers to this topic? (y/n) [default: y]: " subscribe_phones
    subscribe_phones=${subscribe_phones:-y}
    
    if [ "$subscribe_phones" = "y" ]; then
        while true; do
            echo ""
            read -p "Enter phone number in E.164 format (e.g., +12345678900) or press Enter to finish: " phone_number
            
            if [ -z "$phone_number" ]; then
                break
            fi
            
            print_info "Subscribing $phone_number to topic..."
            
            aws sns subscribe \
                --topic-arn "$TOPIC_ARN" \
                --protocol sms \
                --notification-endpoint "$phone_number" \
                --region "$AWS_REGION"
            
            print_success "Subscribed $phone_number"
        done
    fi
}

#######################################
# Function to test SMS delivery
# Optionally sends a test message.
# If a topic was created in this run it publishes to the topic (all
# subscribers receive it) and the phone number entered is not used;
# otherwise it sends a direct SMS to that number. AWS output is discarded.
# Globals:
#   AWS_REGION, TOPIC_ARN (read). MESSAGE, send_test, test_phone (set).
# Arguments:
#   None
# Outputs:
#   Writes prompts and status to stdout; reads stdin.
# Returns:
#   0; a failed publish exits the script (via `set -e`) with no error text
#   because the aws output is redirected to /dev/null.
#######################################
test_sms() {
    echo ""
    read -p "Send a test SMS? (y/n) [default: y]: " send_test
    send_test=${send_test:-y}
    
    if [ "$send_test" != "y" ]; then
        print_info "Skipping SMS test"
        return 0
    fi
    
    echo ""
    read -p "Enter phone number to test (E.164 format, e.g., +12345678900): " test_phone
    
    if [ -z "$test_phone" ]; then
        print_warning "No phone number provided, skipping test"
        return 0
    fi
    
    print_info "Sending test SMS to $test_phone..."
    
    MESSAGE="Test message from Tempest HomeKit Bridge alarm system. Setup successful!"
    
    if [ -n "$TOPIC_ARN" ]; then
        # Publish to topic if it exists
        aws sns publish \
            --topic-arn "$TOPIC_ARN" \
            --message "$MESSAGE" \
            --region "$AWS_REGION" &> /dev/null
        
        print_success "Test message published to topic"
        print_info "All topic subscribers should receive the message"
    else
        # Send directly to phone number
        aws sns publish \
            --phone-number "$test_phone" \
            --message "$MESSAGE" \
            --region "$AWS_REGION" &> /dev/null
        
        print_success "Test SMS sent to $test_phone"
    fi
    
    echo ""
    print_info "Check if the message was delivered"
    print_warning "If not received, you may be in sandbox mode - see next steps below"
}

#######################################
# Function to display final summary
# Prints the configuration summary and follow-up steps.
# Globals:
#   AWS_REGION, SMS_TYPE, TOPIC_ARN (read).
# Arguments:
#   None
# Outputs:
#   Writes the summary to stdout.
# Returns:
#   0.
#######################################
display_summary() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "                    AWS SNS SETUP COMPLETE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    print_success "AWS SNS is configured for production SMS"
    echo ""
    echo "Configuration Summary:"
    echo "  Region: $AWS_REGION"
    echo "  SMS Type: ${SMS_TYPE:-Not set}"
    [ -n "$TOPIC_ARN" ] && echo "  Topic ARN: $TOPIC_ARN"
    echo ""
    echo "Next Steps:"
    echo ""
    echo "1. VERIFY YOUR .ENV FILE:"
    echo "   Your .env should have the APPLICATION RUNTIME USER credentials:"
    echo "   - AWS_ACCESS_KEY_ID (IAM user with SNS send permissions)"
    echo "   - AWS_SECRET_ACCESS_KEY (from that IAM user)"
    echo "   - AWS_REGION (${AWS_REGION})"
    [ -n "$TOPIC_ARN" ] && echo "   - AWS_SNS_TOPIC_ARN (updated by this script)"
    echo ""
    echo "   NOTE: These credentials are DIFFERENT from the admin credentials"
    echo "         you used to run this script. The app needs its own IAM user"
    echo "         with sns:Publish permission only."
    echo ""
    echo "2. CONFIGURE YOUR ALARM JSON:"
    echo "   No provider config needed in JSON - all in .env!"
    echo "   Just add SMS channel to alarms with phone numbers:"
    echo '   {'
    echo '     "type": "sms",'
    echo '     "sms": {'
    echo '       "to": ["+12345678900"],'
    echo '       "message": "⚠️ {{alarm_name}}: {{alarm_description}}"'
    echo '     }'
    echo '   }'
    echo ""
    echo "3. PRODUCTION CONSIDERATIONS:"
    echo "   - Request spending limit increase if needed (currently \$1/month default)"
    echo "   - Move out of sandbox by following AWS verification process"
    echo "   - Consider using an SNS topic for easier subscriber management"
    echo "   - Monitor usage in AWS SNS Console"
    echo ""
    echo "4. SANDBOX MODE EXIT (if in sandbox):"
    echo "   AWS SNS starts in sandbox mode - can only send to verified numbers"
    echo "   To exit sandbox:"
    echo "   a. Go to SNS Console > Text messaging (SMS) > Sandbox destination phone numbers"
    echo "   b. Verify phone numbers OR"
    echo "   c. Request production access through AWS Support case"
    echo ""
    echo "5. RUN YOUR ALARM SYSTEM:"
    echo "   ./tempest-homekit-go --alarms @tempest-alarms.json"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}

#######################################
# Main execution
# Runs every setup step in order.
# Globals:
#   None directly; see the individual steps.
# Arguments:
#   $@ - Accepted but ignored.
# Outputs:
#   Writes the banner and each step's output to stdout.
# Returns:
#   0 on success; exits early as described by the steps.
#######################################
main() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "           AWS SNS Production SMS Setup for Tempest HomeKit"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # Run setup steps
    load_env
    check_aws_cli
    verify_credentials
    check_permissions
    
    echo ""
    get_sms_attributes
    
    configure_production_sms
    request_limit_increase
    create_topic
    test_sms
    
    # Show final summary
    display_summary
}

# Run main function
main "$@"
