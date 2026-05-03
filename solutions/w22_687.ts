// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/security/ReentrancyGuard.sol";

contract GitHubBounty is Ownable, ReentrancyGuard {
    IERC20 public bountyToken;
    uint256 public constant BOUNTY_AMOUNT = 100 * 10**18; // 100 tokens
    address public constant PAYMENT_WALLET = 0xTU8NBT5iGyMNkLwWmWmgy7tFMbKnafLHcu;

    struct Issue {
        uint256 id;
        string title;
        string repo;
        address solver;
        bool completed;
        uint256 timestamp;
    }

    mapping(uint256 => Issue) public issues;
    uint256 public issueCount;

    event IssueCreated(uint256 indexed id, string title, string repo);
    event IssueSolved(uint256 indexed id, address solver);
    event BountyClaimed(uint256 indexed id, address solver, uint256 amount);

    constructor(address _token) {
        bountyToken = IERC20(_token);
    }

    function createIssue(string memory _title, string memory _repo) external onlyOwner {
        issueCount++;
        issues[issueCount] = Issue({
            id: issueCount,
            title: _title,
            repo: _repo,
            solver: address(0),
            completed: false,
            timestamp: block.timestamp
        });
        emit IssueCreated(issueCount, _title, _repo);
    }

    function solveIssue(uint256 _issueId) external {
        require(!issues[_issueId].completed, "Issue already solved");
        require(issues[_issueId].solver == address(0), "Issue already assigned");
        
        issues[_issueId].solver = msg.sender;
        issues[_issueId].completed = true;
        
        emit IssueSolved(_issueId, msg.sender);
    }

    function claimBounty(uint256 _issueId) external nonReentrant {
        Issue storage issue = issues[_issueId];
        require(issue.completed, "Issue not solved");
        require(issue.solver == msg.sender, "Not the solver");
        require(bountyToken.balanceOf(address(this)) >= BOUNTY_AMOUNT, "Insufficient balance");
        
        issue.solver = address(0);
        bountyToken.transfer(msg.sender, BOUNTY_AMOUNT);
        
        emit BountyClaimed(_issueId, msg.sender, BOUNTY_AMOUNT);
    }

    function withdrawTokens() external onlyOwner {
        uint256 balance = bountyToken.balanceOf(address(this));
        require(balance > 0, "No tokens to withdraw");
        bountyToken.transfer(owner(), balance);
    }

    function getIssue(uint256 _issueId) external view returns (Issue memory) {
        return issues[_issueId];
    }

    function getAllIssues() external view returns (Issue[] memory) {
        Issue[] memory allIssues = new Issue[](issueCount);
        for (uint256 i = 1; i <= issueCount; i++) {
            allIssues[i-1] = issues[i];
        }
        return allIssues;
    }
}
